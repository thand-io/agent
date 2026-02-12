package main

import (
	"log"

	"github.com/thand-io/agent/tools/generate-iam-dataset/providers"
)

func main() {
	if err := providers.GenerateAWSFlatBuffers(); err != nil {
		log.Fatal(err)
	}
	if err := providers.GenerateGCPFlatBuffers(); err != nil {
		log.Fatal(err)
	}
	if err := providers.GenerateAzureFlatBuffers(); err != nil {
		log.Fatal(err)
	}
}
