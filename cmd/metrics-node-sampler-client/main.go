// Copyright 2023 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// metrics-node-sampler-client is a simple client for the metrics-node-sampler's
// protocol buffer API.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"sigs.k8s.io/usage-metrics-collector/pkg/sampler/api"
)

func main() {
	address := flag.String("a", "localhost:8080", "address of the metrics-node-sampler")
	flag.Parse()

	// create connection
	conn, err := grpc.Dial(*address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()
	metricsClient := api.NewMetricsClient(conn)
	resp, err := metricsClient.ListMetrics(context.Background(), &api.ListMetricsRequest{})
	if err != nil {
		log.Fatal(err)
	}

	bs, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s\n", bs)
}
