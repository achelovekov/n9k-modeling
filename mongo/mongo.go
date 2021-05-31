package mongo

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DeviceDME struct {
	DeviceName    string                 `bson:"DeviceName"`
	DeviceDMEData map[string]interface{} `bson:"DeviceDMEData"`
}

type MongoDBMetaData struct {
	DBName         string
	CollectionName string
	URL            string
}

func UpdateOne(ctx context.Context, collection *mongo.Collection, filterFieldName string, filterFieldValue interface{}, updateFieldName string, updateFieldValue interface{}) {
	opts := options.Update().SetUpsert(true)
	filter := bson.D{{filterFieldName, filterFieldValue}}
	update := bson.D{{"$set", bson.D{{updateFieldName, updateFieldValue}}}}

	result, err := collection.UpdateOne(ctx, filter, update, opts)

	if err != nil {
		log.Println(err)
	}
	if result.MatchedCount != 0 {
		fmt.Println("matched and replaced an existing document")
		return
	}
	if result.UpsertedCount != 0 {
		fmt.Printf("inserted a new document with ID %v\n", result.UpsertedID)
	}
}

func InsertOne(ctx context.Context, collection *mongo.Collection, document interface{}) (interface{}, error) {
	res, err := collection.InsertOne(ctx, document)
	if err != nil {
		return nil, err
	}
	id := res.InsertedID

	return id, nil
}

func FindOne(ctx context.Context, collection *mongo.Collection, filterFieldName string, filterFieldValue interface{}) (bson.M, error) {
	var result bson.M

	filter := bson.D{{filterFieldName, filterFieldValue}}

	err := collection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, err
		}
		return nil, err
	}

	return result, nil
}
