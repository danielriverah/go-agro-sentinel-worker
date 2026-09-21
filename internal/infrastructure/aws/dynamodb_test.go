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
		{ProduccionID: 1001, Estatus: "OPEN", FechaPlantacion: "2026-01-01", DiasProduccion: 90, Folio: "F001"},
		{ProduccionID: 1002, Estatus: "CLOSED", FechaPlantacion: "2026-01-15", DiasProduccion: 120, Folio: "F002"},
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
			if p.Estatus != "OPEN" {
				t.Errorf("production 1001: Estatus = %q, want OPEN", p.Estatus)
			}
		}
		if p.ProduccionID == 1002 {
			t.Errorf("production 1002 is CLOSED and should not be returned")
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
	createTable(t, rawClient, tableName, "clave")

	ctx := context.Background()

	// Key schema: id (PK) = "PROD#<produccion_id>", clave (SK) = scene name.
	scenes := []DynamoScene{
		{
			ID:         "PROD#2001",
			SceneID:    "scene-a",
			Date:       "2026-02-01",
			CloudCover: 12.5,
		},
		{
			ID:         "PROD#2002",
			SceneID:    "scene-b",
			Date:       "2026-02-02",
			CloudCover: 5.0,
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
}
