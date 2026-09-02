package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func createTable(t *testing.T, client *dynamodb.Client, tableName, hashKey string) {
	t.Helper()
	ctx := context.Background()

	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: &tableName,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: &hashKey, AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: &hashKey, KeyType: types.KeyTypeHash},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		// Table may already exist from a previous run.
		t.Logf("CreateTable(%s): %v (may already exist)", tableName, err)
	}
}

func TestDynamoDBClient_ListActiveProducciones(t *testing.T) {
	cfg := requireLocalstack(t)

	awsCfg, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	rawClient := dynamodb.NewFromConfig(awsCfg)
	tableName := "test_monitoring_producciones"
	createTable(t, rawClient, tableName, "produccion_id")

	ctx := context.Background()

	items := []DynamoProduction{
		{ProduccionID: 1001, Activa: true, Cultivo: "maiz", Ciclo: "2026-A", FechaPlantacion: "2026-01-01", DiasProduccion: 90},
		{ProduccionID: 1002, Activa: false, Cultivo: "soja", Ciclo: "2026-A", FechaPlantacion: "2026-01-15", DiasProduccion: 120},
	}

	for _, item := range items {
		av, err := attributevalue.MarshalMap(item)
		if err != nil {
			t.Fatalf("MarshalMap: %v", err)
		}
		if _, err := rawClient.PutItem(ctx, &dynamodb.PutItemInput{TableName: &tableName, Item: av}); err != nil {
			t.Fatalf("PutItem: %v", err)
		}
	}

	client := &DynamoDBClient{client: rawClient}

	got, err := client.ListActiveProducciones(ctx, tableName)
	if err != nil {
		t.Fatalf("ListActiveProducciones: %v", err)
	}

	found := false
	for _, p := range got {
		if p.ProduccionID == 1001 {
			found = true
			if !p.Activa {
				t.Errorf("production 1001: Activa = false, want true")
			}
		}
		if p.ProduccionID == 1002 {
			t.Errorf("production 1002 is inactive and should not be returned")
		}
	}
	if !found {
		t.Errorf("expected production 1001 in results, got %+v", got)
	}
}

func TestDynamoDBClient_ListEscenas(t *testing.T) {
	cfg := requireLocalstack(t)

	awsCfg, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	rawClient := dynamodb.NewFromConfig(awsCfg)
	tableName := "test_monitoring_escenas"
	createTable(t, rawClient, tableName, "scene_id")

	ctx := context.Background()

	scenes := []DynamoScene{
		{
			SceneID:      "scene-a",
			ProduccionID: 2001,
			Date:         "2026-02-01",
			CloudCover:   12.5,
			STACAssets: map[string]DynamoAsset{
				"B04": {Href: "https://example.com/b04.tif", Resolution: 10},
			},
		},
		{
			SceneID:      "scene-b",
			ProduccionID: 2002,
			Date:         "2026-02-02",
			CloudCover:   5.0,
		},
	}

	for _, scene := range scenes {
		av, err := attributevalue.MarshalMap(scene)
		if err != nil {
			t.Fatalf("MarshalMap: %v", err)
		}
		if _, err := rawClient.PutItem(ctx, &dynamodb.PutItemInput{TableName: &tableName, Item: av}); err != nil {
			t.Fatalf("PutItem: %v", err)
		}
	}

	client := &DynamoDBClient{client: rawClient}

	got, err := client.ListEscenas(ctx, tableName, 2001)
	if err != nil {
		t.Fatalf("ListEscenas: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("ListEscenas: got %d scenes, want 1 (%+v)", len(got), got)
	}
	if got[0].SceneID != "scene-a" {
		t.Errorf("SceneID = %q, want scene-a", got[0].SceneID)
	}
	asset, ok := got[0].STACAssets["B04"]
	if !ok {
		t.Fatalf("expected STAC asset B04, got %+v", got[0].STACAssets)
	}
	if asset.Resolution != 10 {
		t.Errorf("asset.Resolution = %d, want 10", asset.Resolution)
	}
}
