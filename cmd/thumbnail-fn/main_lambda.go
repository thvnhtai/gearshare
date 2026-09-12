//go:build lambda

// Build with `go build -tags lambda` (see deployments/docker/Dockerfile.thumbnail-fn's
// lambda stage) to produce the real AWS Lambda entrypoint. Excluded from the
// default build so `go build ./...` (and the local worker binary,
// main_worker.go) doesn't pull in aws-lambda-go's runtime loop for a target
// that isn't running as a Lambda.
package main

import (
	"context"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(func(ctx context.Context, req ThumbnailRequest) (*ThumbnailResponse, error) {
		return Handler(req)
	})
}
