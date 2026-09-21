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

// DynamoProduction mirrors a production item stored in monitoring_producciones.
// pbox is NOT read from DynamoDB — it is derived from the MySQL polygon.
// cultivo, ciclo, articulo_id, centro_costo_id and nombre_rancho are enriched
// from MySQL (producciones JOIN articulos/centros_costos).
type DynamoProduction struct {
	ProduccionID          int64  `dynamodbav:"produccion_id"`
	Folio                 string `dynamodbav:"folio"`
	Estatus               string `dynamodbav:"estatus"`
	FechaPlantacion       string `dynamodbav:"fecha_siembra"`
	DiasProduccion        int    `dynamodbav:"dias_max_monitoreo"`
	UltimaFechaConsultada string `dynamodbav:"ultima_fecha_consultada"`
}

// DynamoStacAsset is one entry in the stac_assets map of a DynamoScene.
type DynamoStacAsset struct {
	Href       string  `dynamodbav:"href"`
	Resolution float64 `dynamodbav:"resolution"`
}

// DynamoScene mirrors a scene item stored in monitoring_escenas (production_monitoring_detalle).
// Partition key: id (String) = "PROD#<produccion_id>", Sort key: clave (String) = scene name.
//
// stac_assets is the canonical source of band URLs. The legacy flat fields
// (band_blue, band_green, band_nir, band_red) are kept for backwards
// compatibility with older table items that lack stac_assets.
type DynamoScene struct {
	ID      string `dynamodbav:"id"`   // "PROD#<produccion_id>"
	SceneID string `dynamodbav:"clave"` // sort key = scene name
	Date         string                     `dynamodbav:"fecha"`
	CloudCover   float64                    `dynamodbav:"cloud_cover"`
	StacAssets   map[string]DynamoStacAsset `dynamodbav:"stac_assets"`
	// Legacy flat band fields — used when stac_assets is absent.
	BandBlue  string `dynamodbav:"band_blue"`
	BandGreen string `dynamodbav:"band_green"`
	BandNir   string `dynamodbav:"band_nir"`
	BandRed   string `dynamodbav:"band_red"`
}

// BaseBandURL returns the best reference band URL for use as base_bands.
// It prefers stac_assets (in B02 → B03 → B04 → B08 order) and falls back
// to the legacy flat fields.
func (d *DynamoScene) BaseBandURL() string {
	for _, name := range []string{"B02", "B03", "B04", "B08", "B05", "B11", "B12"} {
		if a, ok := d.StacAssets[name]; ok && a.Href != "" {
			return a.Href
		}
	}
	// Legacy fallback.
	for _, v := range []string{d.BandBlue, d.BandGreen, d.BandNir, d.BandRed} {
		if v != "" {
			return v
		}
	}
	return ""
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
// "estatus" attribute equals "OPEN".
func (c *DynamoDBClient) ListActiveProducciones(ctx context.Context, tableName string) ([]DynamoProduction, error) {
	filt := expression.Name("estatus").Equal(expression.Value("OPEN"))
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

// ListEscenas queries tableName for all scenes belonging to produccionID.
// The table key schema is: id (String, PK) = "PROD#<produccion_id>", clave (String, SK).
func (c *DynamoDBClient) ListEscenas(ctx context.Context, tableName string, produccionID int64) ([]DynamoScene, error) {
	pk := fmt.Sprintf("PROD#%d", produccionID)

	keyCond := expression.Key("id").Equal(expression.Value(pk))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "building key condition expression", Wrapped: err}
	}

	var results []DynamoScene

	paginator := dynamodb.NewQueryPaginator(c.client, &dynamodb.QueryInput{
		TableName:                 awssdk.String(tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: fmt.Sprintf("querying table %s", tableName), Wrapped: err}
		}

		var pageItems []DynamoScene
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageItems); err != nil {
			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "unmarshaling escenas", Wrapped: err}
		}

		results = append(results, pageItems...)
	}

	return results, nil
}

// CloseProduccion sets estatus = "CLOSED" on the DynamoDB item identified by
// produccionID (partition key, Number) and folio (sort key, String).
func (c *DynamoDBClient) CloseProduccion(ctx context.Context, tableName string, produccionID int64, folio string) error {
	upd := expression.Set(expression.Name("estatus"), expression.Value("CLOSED"))
	expr, err := expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		return fmt.Errorf("building update expression: %w", err)
	}

	key, err := attributevalue.MarshalMap(map[string]interface{}{
		"produccion_id": produccionID,
		"folio":         folio,
	})
	if err != nil {
		return fmt.Errorf("marshaling key: %w", err)
	}

	_, err = c.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 awssdk.String(tableName),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		return fmt.Errorf("closing produccion %d in dynamodb: %w", produccionID, err)
	}
	return nil
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
