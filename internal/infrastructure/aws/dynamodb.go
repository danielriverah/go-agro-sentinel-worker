package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"agro-sentinel-worker/internal/domain"
)

// DynamoAsset is a single STAC asset entry stored on a DynamoDB scene item.
type DynamoAsset struct {
	Href       string `dynamodbav:"href"`
	Resolution int    `dynamodbav:"resolution"`
}

// DynamoProduction mirrors a production item stored in DynamoDB.
type DynamoProduction struct {
	ProduccionID    int64  `dynamodbav:"produccion_id"`
	Activa          bool   `dynamodbav:"activa"`
	Cultivo         string `dynamodbav:"cultivo"`
	Ciclo           string `dynamodbav:"ciclo"`
	FechaPlantacion string `dynamodbav:"fecha_plantacion"`
	DiasProduccion  int    `dynamodbav:"dias_produccion"`
}

// DynamoScene mirrors a scene item stored in DynamoDB.
type DynamoScene struct {
	SceneID      string                 `dynamodbav:"scene_id"`
	ProduccionID int64                  `dynamodbav:"produccion_id"`
	Date         string                 `dynamodbav:"date"`
	CloudCover   float64                `dynamodbav:"cloud_cover"`
	STACAssets   map[string]DynamoAsset `dynamodbav:"stac_assets"`
}

// DynamoDBClient wraps the AWS SDK v2 DynamoDB client with the scan
// operations needed by the worker.
type DynamoDBClient struct {
	client *dynamodb.Client
}

// NewDynamoDBClient builds a DynamoDBClient from an already-resolved AWS SDK config.
func NewDynamoDBClient(awsCfg awssdk.Config) *DynamoDBClient {
	return &DynamoDBClient{client: dynamodb.NewFromConfig(awsCfg)}
}

// ListActiveProducciones scans tableName and returns every item whose
// "activa" attribute is true.
func (c *DynamoDBClient) ListActiveProducciones(ctx context.Context, tableName string) ([]DynamoProduction, error) {
	filt := expression.Name("activa").Equal(expression.Value(true))
	expr, err := expression.NewBuilder().WithFilter(filt).Build()
	if err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "building filter expression", Wrapped: err}
	}

	var results []DynamoProduction

	paginator := dynamodb.NewScanPaginator(c.client, &dynamodb.ScanInput{
		TableName:                 awssdk.String(tableName),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: fmt.Sprintf("scanning table %s", tableName), Wrapped: err}
		}

		var pageItems []DynamoProduction
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageItems); err != nil {
			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "unmarshaling producciones", Wrapped: err}
		}

		results = append(results, pageItems...)
	}

	return results, nil
}

// ListEscenas scans tableName and returns every item whose "produccion_id"
// attribute equals produccionID.
func (c *DynamoDBClient) ListEscenas(ctx context.Context, tableName string, produccionID int64) ([]DynamoScene, error) {
	filt := expression.Name("produccion_id").Equal(expression.Value(produccionID))
	expr, err := expression.NewBuilder().WithFilter(filt).Build()
	if err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "building filter expression", Wrapped: err}
	}

	var results []DynamoScene

	paginator := dynamodb.NewScanPaginator(c.client, &dynamodb.ScanInput{
		TableName:                 awssdk.String(tableName),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: fmt.Sprintf("scanning table %s", tableName), Wrapped: err}
		}

		var pageItems []DynamoScene
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageItems); err != nil {
			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "unmarshaling escenas", Wrapped: err}
		}

		results = append(results, pageItems...)
	}

	return results, nil
}

// DescribeTable checks if a table exists and is accessible.
func (c *DynamoDBClient) DescribeTable(ctx context.Context, tableName string) error {
	_, err := c.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: awssdk.String(tableName),
	})
	if err != nil {
		return fmt.Errorf("checking dynamodb table %s: %w", tableName, err)
	}
	return nil
}
