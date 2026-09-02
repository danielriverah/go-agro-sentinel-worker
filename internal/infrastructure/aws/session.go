package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"agro-sentinel-worker/internal/config"
)

// NewSession creates an AWS SDK v2 config from the application's AWS
// configuration. When cfg.Endpoint is set (e.g. for localstack), all
// service clients created from the returned config will resolve to that
// endpoint instead of the real AWS endpoints.
func NewSession(cfg config.AWSConfig) (awssdk.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.Endpoint != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(cfg.Endpoint))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return awssdk.Config{}, err
	}

	return awsCfg, nil
}
