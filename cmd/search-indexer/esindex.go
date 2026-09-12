package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/sony/gobreaker"

	gearsharev1 "github.com/thvnhtai/gearshare/api/gen/gearshare/v1"
)

const indexName = "gear_listings"

// esDocument is the denormalized shape actually indexed — everything a
// search result needs to render, so the monolith's search path never has
// to join back to MySQL for display fields (see internal/search.Service).
type esDocument struct {
	ListingID        int64   `json:"listing_id"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	PricePerDayCents int64   `json:"price_per_day_cents"`
	CategoryName     string  `json:"category_name"`
	OwnerName        string  `json:"owner_name"`
	Status           string  `json:"status"`
	AvgRating        float64 `json:"avg_rating"`
}

// Index wraps the Elasticsearch client behind a circuit breaker — the
// requirement checklist's "circuit breaker... inside the indexer itself"
// (as opposed to internal/middleware/breaker.go's breaker around the
// monolith's gRPC call to this service). If Elasticsearch is down, indexing
// calls fail fast instead of piling up retries against a dead dependency.
type Index struct {
	client  *elasticsearch.Client
	breaker *gobreaker.CircuitBreaker
}

func NewIndex(addresses []string, breaker *gobreaker.CircuitBreaker) (*Index, error) {
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: addresses})
	if err != nil {
		return nil, fmt.Errorf("esindex: new client: %w", err)
	}
	return &Index{client: client, breaker: breaker}, nil
}

func (idx *Index) EnsureIndex(ctx context.Context) error {
	res, err := idx.client.Indices.Exists([]string{indexName}, idx.client.Indices.Exists.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("esindex: check index exists: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		return nil
	}

	create, err := idx.client.Indices.Create(indexName, idx.client.Indices.Create.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("esindex: create index: %w", err)
	}
	defer create.Body.Close()
	if create.IsError() {
		return fmt.Errorf("esindex: create index: %s", create.String())
	}
	return nil
}

func (idx *Index) Upsert(ctx context.Context, doc esDocument) error {
	_, err := idx.breaker.Execute(func() (interface{}, error) {
		body, err := json.Marshal(doc)
		if err != nil {
			return nil, err
		}
		req := esapi.IndexRequest{
			Index:      indexName,
			DocumentID: strconv.FormatInt(doc.ListingID, 10),
			Body:       bytes.NewReader(body),
			Refresh:    "true", // demo-scale only: makes the doc immediately searchable, at a real indexing-throughput cost
		}
		res, err := req.Do(ctx, idx.client)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.IsError() {
			return nil, fmt.Errorf("esindex: index doc %d: %s", doc.ListingID, res.String())
		}
		return nil, nil
	})
	return err
}

func (idx *Index) Delete(ctx context.Context, listingID int64) error {
	_, err := idx.breaker.Execute(func() (interface{}, error) {
		req := esapi.DeleteRequest{Index: indexName, DocumentID: strconv.FormatInt(listingID, 10)}
		res, err := req.Do(ctx, idx.client)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		return nil, nil
	})
	return err
}

func (idx *Index) Search(ctx context.Context, query string, limit int) ([]*gearsharev1.SearchResult, error) {
	result, err := idx.breaker.Execute(func() (interface{}, error) {
		q := map[string]interface{}{
			"size": limit,
			"query": map[string]interface{}{
				"multi_match": map[string]interface{}{
					"query":  query,
					"fields": []string{"title^2", "description"},
				},
			},
		}
		body, err := json.Marshal(q)
		if err != nil {
			return nil, err
		}
		res, err := idx.client.Search(
			idx.client.Search.WithContext(ctx),
			idx.client.Search.WithIndex(indexName),
			idx.client.Search.WithBody(bytes.NewReader(body)),
		)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.IsError() {
			return nil, fmt.Errorf("esindex: search: %s", res.String())
		}

		var parsed struct {
			Hits struct {
				Hits []struct {
					Source esDocument `json:"_source"`
				} `json:"hits"`
			} `json:"hits"`
		}
		if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
			return nil, err
		}

		results := make([]*gearsharev1.SearchResult, 0, len(parsed.Hits.Hits))
		for _, h := range parsed.Hits.Hits {
			results = append(results, &gearsharev1.SearchResult{
				ListingId:        h.Source.ListingID,
				Title:            h.Source.Title,
				PricePerDayCents: h.Source.PricePerDayCents,
				CategoryName:     h.Source.CategoryName,
				AvgRating:        h.Source.AvgRating,
			})
		}
		return results, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]*gearsharev1.SearchResult), nil
}
