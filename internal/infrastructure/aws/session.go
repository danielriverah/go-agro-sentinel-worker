package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"agro-sentinel-worker/internal/config"
)

// NewDynamoDBSession builds an AWS config specifically for DynamoDB.
// If cfg.AccessKeyID is set, explicit static credentials are used (real AWS).
// If cfg.Endpoint is set, requests go there instead of the real AWS endpoint.
// If neither is set it falls back to the default credential chain (env vars / instance profile).
//
// When cfg.AccessKeyID is set and cfg.Endpoint is empty, the config is built
// directly (bypassing LoadDefaultConfig) so that AWS_ENDPOINT_URL pointing to
// LocalStack does not leak into this DynamoDB session.
func NewDynamoDBSession(cfg config.DynamoDBConfig) (awssdk.Config, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	// Real AWS: explicit credentials, no custom endpoint — build directly
	// so env vars like AWS_ENDPOINT_URL (LocalStack) are not inherited.
	if cfg.AccessKeyID != "" && cfg.Endpoint == "" {
		return awssdk.Config{
			Region:      region,
			Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		}, nil
	}

	opts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}
	if cfg.AccessKeyID != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}
	if cfg.Endpoint != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(cfg.Endpoint))
	}
	return awsconfig.LoadDefaultConfig(context.Background(), opts...)
}

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

// NewS3Session builds an AWS config specifically for S3 with independent region and credentials.
// Uses cfg.S3.Region instead of the general AWS region, allowing S3 to be in a
// different region than other services (e.g., S3 in us-west-2, DynamoDB in us-east-1).
// If cfg.S3.Region is empty, falls back to cfg.AWS.Region.
// Also uses S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY if configured.
func NewS3Session(s3Cfg config.S3Config, awsCfg config.AWSConfig) (awssdk.Config, error) {
	region := s3Cfg.Region
	if region == "" {
		region = awsCfg.Region
	}
	if region == "" {
		region = "us-east-1"
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	// Use explicit S3 credentials if provided (from S3_ACCESS_KEY_ID/S3_SECRET_ACCESS_KEY)
	// Fall back to AWS credentials if not set
	accessKeyID := s3Cfg.AccessKeyID
	secretAccessKey := s3Cfg.SecretAccessKey
	if accessKeyID == "" && awsCfg.AccessKeyID != "" {
		accessKeyID = awsCfg.AccessKeyID
	}
	if secretAccessKey == "" && awsCfg.SecretAccessKey != "" {
		secretAccessKey = awsCfg.SecretAccessKey
	}

	if accessKeyID != "" && secretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		))
	}

	if awsCfg.Endpoint != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(awsCfg.Endpoint))
	}

	return awsconfig.LoadDefaultConfig(context.Background(), opts...)
}
