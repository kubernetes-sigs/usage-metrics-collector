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

package cmds

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"google.golang.org/api/option"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/usage-metrics-collector/pkg/collector"
	commonlog "sigs.k8s.io/usage-metrics-collector/pkg/log"
)

func Execute(version, commit, date string) {
	if err := RootCmd.Execute(); err != nil {
		log.Error(err, "failed to run")
		os.Exit(1)
	}
}

type Options struct {
	frequency          time.Duration
	delete             bool
	projectID          string
	bqDataSetID        string
	bqTableID          string
	gcsBucket          string
	gcsFolder          string
	gcsKeepFolderParts []int
	globs              []string
}

func init() {
	RootCmd.Flags().StringVar(&metricsAddr, "metrics-addr", ":8585", "bind address for hosting metrics.")

	RootCmd.Flags().DurationVar(&options.frequency, "frequency", 0, "frequency to check for new files.")
	RootCmd.Flags().BoolVar(&options.delete, "delete", true, "if true, delete files that have been archived.")

	// BigQuery options
	RootCmd.Flags().StringVar(&options.projectID, "project", "", "bigquery project.")
	RootCmd.Flags().StringVar(&options.bqDataSetID, "bq-dataset", "", "bigquery dataset.")
	RootCmd.Flags().StringVar(&options.bqTableID, "bq-table", "", "bigquery table.")

	// GCS options
	RootCmd.Flags().IntSliceVar(&options.gcsKeepFolderParts, "gcs-keep-folder-parts", []int{}, "keep these parts of the file directory.")
	RootCmd.Flags().StringVar(&options.gcsFolder, "gcs-folder", "", "gcs folder to put files into.  note: may be templatized with the time.")
	RootCmd.Flags().StringVar(&options.gcsBucket, "gcs-bucket", "", "gcs bucket.")
}

var (
	options Options
	log     = commonlog.Log.WithName("umc-archiver")

	metricsAddr string

	bigQueryInsertSuccesses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "usage_metrics_collector_bigquery_insert_successes_total",
		Help: "The total number of files successfully copied",
	})

	archiveFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "usage_metrics_collector_bigquery_insert_failures_total",
		Help: "The total number of failures when copying files",
	}, []string{"reason"})

	RootCmd = &cobra.Command{
		Use:   "umc-archiver [DIRECTORY_PATH]",
		Short: "Insert local sample files into BigQuery.",
		Long: `
# continuously insert metrics into BigQuery
umc-archiver /tmp/metrics-prometheus-collector-samples/ --frequency 1m
`,
		Args:    cobra.MinimumNArgs(1),
		PreRunE: preRunE,
		RunE:    runE,
	}
)

func preRunE(cmd *cobra.Command, args []string) error {
	if metricsAddr != "" {
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			err := http.ListenAndServe(metricsAddr, nil)
			if err != nil {
				log.Error(err, "unable to start metrics server")
				os.Exit(1)
			}
		}()
	}
	options.globs = args
	options.gcsFolder = time.Now().Format(options.gcsFolder)
	return nil
}

func runE(cmd *cobra.Command, args []string) error {
	if options.frequency == 0 { // run once and exit
		if err := archiveFiles(); err != nil {
			// we aren't running continuously, so don't use the metrics
			// to report errors
			return err
		}
		return nil
	}
	ticker := time.NewTicker(options.frequency)
	for {
		if err := archiveFiles(); err != nil {
			log.Error(err, "failed to insert files into BigQuery")
			os.Exit(1)
		}
		<-ticker.C
	}
}

func copyGCS(filename string, filecontents *bytes.Buffer, ctx context.Context, client *storage.Client) error {
	var folder string
	if options.gcsBucket != "" {
		parts := strings.Split(filename, string("/"))
		log.Info("splitting filename for gcs folder", "parts", parts, "keep", options.gcsKeepFolderParts)
		for _, i := range options.gcsKeepFolderParts {
			folder = path.Join(folder, parts[i])
		}
	}
	gcsObj := strings.ReplaceAll(options.gcsFolder, "PATH", folder) // copy the original path
	gcsObj = gcsObj + path.Base(filename) + ".gz"                   // add the filename to the object name
	log.Info("uploading to gcs", "filename", gcsObj)

	o := client.Bucket(options.gcsBucket).Object(gcsObj) // use path.Join instead of filepath.Join
	o = o.If(storage.Conditions{DoesNotExist: true})

	// write the file to the gcp bucket
	writeGCS := o.NewWriter(ctx)
	writeZip := gzip.NewWriter(writeGCS)
	if _, err := io.Copy(writeZip, filecontents); err != nil {
		archiveFailures.WithLabelValues("gcs-copy-zip").Inc()
		log.Error(err, "failed to copy sample file", "filename", filename)
		return err
	}
	if err := writeZip.Close(); err != nil {
		archiveFailures.WithLabelValues("gcs-close-zip").Inc()
		log.Error(err, "failed to write compressed file", "filename", filename)
		return err
	}

	if err := writeGCS.Close(); err != nil {
		archiveFailures.WithLabelValues("gcs-close").Inc()
		log.Error(err, "failed to complete sample upload to cloud-storage", "filename", filename)
		return nil
	}
	return nil
}

func loadBigQuery(filename string, filecontents *bytes.Buffer, ctx context.Context, client *bigquery.Client) error {
	// load into BigQery
	log.Info("loading into BigQuery", "filename", filename)
	// infer the schema -- check for the labels
	labels := sets.NewString()
	for _, s := range strings.Split(filecontents.String(), "\n") {
		var l collector.JSONLine
		json.Unmarshal([]byte(s), &l)
		for k := range l.Labels {
			labels.Insert(k)
		}
	}
	labelsSchema := bigquery.Schema{}
	for _, k := range labels.List() {
		labelsSchema = append(labelsSchema, &bigquery.FieldSchema{Name: k, Type: bigquery.StringFieldType})
	}
	schema := []*bigquery.FieldSchema{
		{Name: "metricName", Type: bigquery.StringFieldType, Required: true},
		{Name: "value", Type: bigquery.FloatFieldType},
		{Name: "timestamp", Type: bigquery.TimestampFieldType},
		{Name: "labels", Type: bigquery.RecordFieldType, Schema: labelsSchema},
	}

	// load the data into BigQuery
	rs := bigquery.NewReaderSource(filecontents)
	rs.FileConfig.SourceFormat = bigquery.JSON
	rs.FileConfig.Schema = schema

	loader := client.Dataset(options.bqDataSetID).Table(options.bqTableID).LoaderFrom(rs)
	loader.SchemaUpdateOptions = []string{"ALLOW_FIELD_ADDITION"}
	log.Info("inserting items")
	j, err := loader.Run(ctx)
	if err != nil {
		archiveFailures.WithLabelValues("bq-run-load").Inc()
		return err
	}
	js, err := j.Wait(ctx)
	if err != nil {
		archiveFailures.WithLabelValues("bq-wait-load").Inc()
		return err
	}
	if js.Err() != nil {
		archiveFailures.WithLabelValues("bq-job-status").Inc()
		log.Error(js.Err(), "failed waiting on job status")
		for _, err := range js.Errors {
			log.Error(err, "load BigQuery data failed")
		}
		return err
	}
	log.Info("finished loading file", "filename", filename)
	return nil
}

func archiveFiles() error {
	ctx := context.Background()
	bqClient, err := bigquery.NewClient(ctx, options.projectID, GCPClientOptionProvider(ctx)...)
	if err != nil {
		archiveFailures.WithLabelValues("bq-create-client").Inc()
		return fmt.Errorf("bigquery.NewClient: %w", err)
	}
	defer bqClient.Close()

	gcsClient, err := storage.NewClient(ctx, GCPClientOptionProvider(ctx)...)
	if err != nil {
		archiveFailures.WithLabelValues("gcs-create-client").Inc()
		return fmt.Errorf("storage.NewClient: %w", err)
	}
	defer gcsClient.Close()

	for _, g := range options.globs {
		files, err := filepath.Glob(g)
		if err != nil {
			archiveFailures.WithLabelValues("create-glob").Inc()
			return err
		}
		for _, f := range files {
			if err := func() error {
				ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
				defer cancel()

				// read the file
				log.Info("reading file", "filename", f)
				b, err := os.ReadFile(f)
				if err != nil {
					archiveFailures.WithLabelValues("read-file").Inc()
					return err
				}
				b = []byte(strings.TrimSpace(string(b)))
				if len(b) == 0 {
					return nil // empty file
				}

				// copy to GCS
				if options.gcsBucket != "" {
					err = copyGCS(f, bytes.NewBuffer(b), ctx, gcsClient)
					if err != nil {
						return err
					}
				}

				// load into BigQuery
				if options.bqTableID != "" {
					err = loadBigQuery(f, bytes.NewBuffer(b), ctx, bqClient)
					if err != nil {
						return err
					}
				}

				// delete the file
				err = os.Remove(f)
				if err != nil {
					archiveFailures.WithLabelValues("delete-file").Inc()
					return err
				}

				bigQueryInsertSuccesses.Inc()
				return nil
			}(); err != nil {
				return err
			}
		}
	}
	return nil
}

// GCPClientOptionProvider may be overridden to specify custom options
var GCPClientOptionProvider func(context.Context) []option.ClientOption = func(
	context.Context) []option.ClientOption {
	return []option.ClientOption{}
}
