package main

import (
	"context"
	"flag"
	"log"
	"sync"
	"time"

	m "n9k-modeling/modeling"

	mo "n9k-modeling/mongo"

	cu "github.com/achelovekov/collectorutils"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	InventoryFile := flag.String("i", "00000", "inventory file to proceess")
	flag.Parse()
	var wg sync.WaitGroup

	Inventory := cu.LoadInventory(*InventoryFile)

	MongoDBMetaData := mo.MongoDBMetaData{
		DBName:         "RawDME",
		CollectionName: "DevicesRawDME",
		URL:            "mongodb://localhost:27017",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(MongoDBMetaData.URL))

	if err != nil {
		log.Println(err)
	}

	collection := client.Database(MongoDBMetaData.DBName).Collection(MongoDBMetaData.CollectionName)

	for _, v := range Inventory {
		wg.Add(1)
		go m.GetRawData(ctx, collection, v, "sys", &wg)
	}

	wg.Wait()

}
