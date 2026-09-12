// Package main implements GearShare's serverless demo: a listing-image
// thumbnail generator. The interesting part is that Handler is exercised by
// TWO different entrypoints in this same binary target — main_lambda.go
// (a real AWS Lambda handler, deployable as-is) and main_worker.go (a local
// RabbitMQ consumer loop) — proving the function itself is genuinely
// FaaS-shaped (stateless, single input/output, no long-lived state) rather
// than a long-running service pretending to be one. See
// docs/architecture.md's "architectural patterns" table.
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // registers PNG decoding with image.Decode

	"golang.org/x/image/draw"
)

type ThumbnailRequest struct {
	ListingID     int64  `json:"listing_id"`
	ImageBase64   string `json:"image_base64"`
	ContentType   string `json:"content_type"` // "image/jpeg" | "image/png"
	MaxDimension  int    `json:"max_dimension"`
}

type ThumbnailResponse struct {
	ListingID         int64  `json:"listing_id"`
	ThumbnailBase64   string `json:"thumbnail_base64"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
}

// Handler decodes the source image, resizes it to fit within MaxDimension
// (preserving aspect ratio) using a simple bilinear scale, and returns the
// result re-encoded as JPEG. Pure function: no filesystem, no network, no
// package-level state — everything a FaaS runtime needs to be able to run
// it on an arbitrary, short-lived worker.
func Handler(req ThumbnailRequest) (*ThumbnailResponse, error) {
	if req.MaxDimension <= 0 {
		req.MaxDimension = 320
	}

	raw, err := base64.StdEncoding.DecodeString(req.ImageBase64)
	if err != nil {
		return nil, fmt.Errorf("thumbnail-fn: decode base64: %w", err)
	}

	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("thumbnail-fn: decode image: %w", err)
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	scale := float64(req.MaxDimension) / float64(max(w, h))
	if scale > 1 {
		scale = 1 // never upscale
	}
	dstW, dstH := int(float64(w)*scale), int(float64(h)*scale)

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("thumbnail-fn: encode jpeg: %w", err)
	}

	return &ThumbnailResponse{
		ListingID:       req.ListingID,
		ThumbnailBase64: base64.StdEncoding.EncodeToString(out.Bytes()),
		Width:           dstW,
		Height:          dstH,
	}, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
