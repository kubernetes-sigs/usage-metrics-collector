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

package collector

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	collectorapi "sigs.k8s.io/usage-metrics-collector/pkg/collector/api"
)

type SampleListBuilder struct {
	Mask          collectorcontrollerv1alpha1.LabelsMask
	SourceType    collectorcontrollerv1alpha1.SourceType
	DirectoryPath string
	Save          bool
	TimeFormat    string
	SampleList    *collectorapi.SampleList
	C             *Collector
}

func (c *Collector) NewAggregatedSampleListBuilder(src collectorcontrollerv1alpha1.SourceType) *SampleListBuilder {
	return c.newSampleListBuilder(src, true)
}

func (c *Collector) NewSampleListBuilder(src collectorcontrollerv1alpha1.SourceType) *SampleListBuilder {
	return c.newSampleListBuilder(src, false)
}

// NewSampleListBuilder returns a SampleBuilder for
func (c *Collector) newSampleListBuilder(src collectorcontrollerv1alpha1.SourceType, createIfNoMatch bool) *SampleListBuilder {
	if c.SaveSamplesLocally == nil {
		return nil
	}
	ts := timestamppb.New(time.Now())
	sb := &SampleListBuilder{
		C:             c,
		SourceType:    src,
		DirectoryPath: c.SaveSamplesLocally.DirectoryPath,
		TimeFormat:    c.SaveSamplesLocally.TimeFormat,
		SampleList:    &collectorapi.SampleList{Timestamp: ts, Type: src.String()},
	}
	// index the label mask to use  for each sample
	for _, s := range c.SaveSamplesLocally.SampleSources {
		if s.Sources.Type == src {
			sb.Mask = s.Mask
			return sb
		}
	}
	if createIfNoMatch {
		return sb
	}
	return nil
}

// NewSample creates a new Sample for an object
func (sb *SampleListBuilder) NewSample(labels LabelsValues) *collectorapi.Sample {
	if sb == nil {
		return nil
	}
	// get the labels
	result := map[string]string{}
	names := sb.C.getLabelNames(sb.Mask)
	values := sb.C.getLabelValues(sb.Mask, labels, names)
	for i := range names {
		result[names[i]] = values[i]
	}

	sample := &collectorapi.Sample{Labels: result}
	sb.SampleList.Items = append(sb.SampleList.Items, sample)
	return sample
}

// AddQuantityValues adds a Metric to the sample with the provided values
func (sb *SampleListBuilder) AddQuantityValues(s *collectorapi.Sample, resourceType collectorcontrollerv1alpha1.ResourceName, source collectorcontrollerv1alpha1.Source, v ...resource.Quantity) {
	if sb == nil {
		return
	}

	m := &collectorapi.Metric{Source: string(source), ResourceType: string(resourceType)}
	s.Values = append(s.Values, m)
	for i := range v {
		m.Values = append(m.Values, v[i].AsApproximateFloat64())
	}
}

// AddHistogramValues adds a Metric to the sample with values in histogram format
func (sb *SampleListBuilder) AddHistogramValues(s *collectorapi.Sample, resourceType collectorcontrollerv1alpha1.ResourceName,
	source collectorcontrollerv1alpha1.Source, buckets collectorcontrollerv1alpha1.ExponentialBuckets, v ...resource.Quantity) {
	if sb == nil {
		return
	}

	m := &collectorapi.Metric{Source: string(source), ResourceType: string(resourceType)}
	s.Values = append(s.Values, m)

	m.Histogram = &collectorapi.ExponentialBuckets{Counts: &collectorapi.ExponentialBucketCounts{}}
	// base = MinBase ^ (2 ^ Compression)
	m.Histogram.Base = math.Pow(buckets.MinimumBase, math.Pow(2, float64(buckets.Compression)))

	// lowerBound of histogram
	// any values below lowerBound are considered zero
	lowerBound := math.Pow(m.Histogram.Base, float64(buckets.ExponentOffset))

	// max value stored in this histogram
	maxVal := math.Inf(-1)
	for i := range v {
		val := v[i].AsApproximateFloat64()
		if val < lowerBound {
			m.Histogram.ZeroCount++
		}
		maxVal = math.Max(maxVal, val)
	}

	m.Histogram.MaxValue = maxVal
	m.Histogram.BaseScale = int32(buckets.Compression)
	m.Histogram.Counts.ExponentOffset = int32(buckets.ExponentOffset)

	// store only maxvalue
	if buckets.SaveMaxOnly || maxVal < lowerBound {
		return
	}

	// max bucket index
	// max index = log(maxVal)/log(base) -  exponentOffset + 1
	maxIndex := int64(math.Round(math.Log(maxVal)/math.Log(m.Histogram.Base))) - buckets.ExponentOffset 

	bucketCounts := make([]int64, maxIndex+1)

	// set histogram bucket counts
	for i := range v {
		val := v[i].AsApproximateFloat64()
		if val < lowerBound {
			continue
		}
		index := int64(math.Round(math.Log(val)/math.Log(m.Histogram.Base))) - buckets.ExponentOffset
		bucketCounts[index]++
	}

	m.Histogram.Counts.BucketCounts = append(m.Histogram.Counts.BucketCounts, bucketCounts...)
}

// AddIntValues adds a Metric to the sample with the provided values
func (sb *SampleListBuilder) AddIntValues(s *collectorapi.Sample, resourceType collectorcontrollerv1alpha1.ResourceName, source collectorcontrollerv1alpha1.Source, v ...int64) {
	if sb == nil {
		return
	}

	m := &collectorapi.Metric{Source: string(source), ResourceType: string(resourceType)}
	s.Values = append(s.Values, m)
	for i := range v {
		m.Values = append(m.Values, float64(v[i]))
	}
}

// SaveSamplesToFile writes the SampleList to a binary file with the timestamp
func (sb *SampleListBuilder) SaveSamplesToFile() error {
	if sb == nil {
		return nil
	}

	if sb.C.SaveSamplesLocally.ExcludeTimestamp {
		sb.SampleList.Timestamp = nil
	}

	if clstr := os.Getenv("CLUSTER_NAME"); clstr != "" {
		sb.SampleList.ClusterName = clstr
	}

	if sb.C.SaveSamplesLocally.SortValues {
		for i := range sb.SampleList.Items {
			item := sb.SampleList.Items[i]
			sort.Slice(item.Values, func(i, j int) bool {
				if item.Values[i].Source != item.Values[j].Source {
					return item.Values[i].Source < item.Values[j].Source
				}
				return item.Values[i].ResourceType < item.Values[j].ResourceType

			})
		}
	}

	b, err := proto.Marshal(sb.SampleList)
	if err != nil {
		return err
	}
	cn := os.Getenv("CLUSTER_NAME")
	var filename string
	if cn != "" {
		filename = filepath.Join(sb.DirectoryPath, sb.SourceType.String(), time.Now().Format(sb.TimeFormat)+"_"+cn+"_"+sb.SourceType.String()+".samplelist.pb")
	} else {
		filename = filepath.Join(sb.DirectoryPath, sb.SourceType.String(), time.Now().Format(sb.TimeFormat)+"_"+sb.SourceType.String()+".samplelist.pb")
	}
	err = os.MkdirAll(filepath.Dir(filename), 0700)
	if err != nil {
		return err
	}

	err = os.WriteFile(filename, b, 0200)
	if err != nil {
		return err
	}
	err = os.Chmod(filename, 0600)
	if err != nil {
		return err
	}
	return nil
}

func (c *Collector) NormalizeForSave(sr *collectorapi.ScrapeResult) error {
	if sr == nil {
		return nil
	}

	if c.SaveSamplesLocally.ExcludeTimestamp {
		for i := range sr.Items {
			sr.Items[i].Timestamp = nil
		}
	}
	if clstr := os.Getenv("CLUSTER_NAME"); clstr != "" {
		for i := range sr.Items {
			sr.Items[i].ClusterName = clstr
		}
	}
	if c.SaveSamplesLocally.SortValues {
		sort.Slice(sr.Items, func(i, j int) bool {
			return sr.Items[i].MetricName < sr.Items[j].MetricName
		})
		for i := range sr.Items {
			items := sr.Items[i].Items
			sort.Slice(items, func(i, j int) bool {
				if items[i].Source != items[j].Source {
					return items[i].Source < items[j].Source
				}
				if items[i].Level != items[j].Level {
					return items[i].Level < items[j].Level
				}
				if items[i].Operation != items[j].Operation {
					return items[i].Operation < items[j].Operation
				}
				var keysi []string
				for k := range items[i].Labels {
					keysi = append(keysi, k)
				}
				sort.Strings(keysi)
				for _, k := range keysi {
					if items[i].Labels[k] != items[j].Labels[k] {
						return items[i].Labels[k] < items[j].Labels[k]
					}
				}
				return false
			})

			for j := range items {
				item := items[j]
				sort.Slice(item.Values, func(i, j int) bool {
					if item.Values[i].Source != item.Values[j].Source {
						return item.Values[i].Source < item.Values[j].Source
					}
					if item.Values[i].ResourceType != item.Values[j].ResourceType {
						return item.Values[i].ResourceType < item.Values[j].ResourceType
					}
					return false
				})
			}
		}
	}
	return nil
}

func (c *Collector) SaveScrapeResultToFile(sr *collectorapi.ScrapeResult) error {
	if sr == nil {
		return nil
	}

	// shard output files so they are more efficient to read
	am := map[string]*collectorapi.ScrapeResult{}
	for _, s := range sr.Items {
		if _, ok := am[s.Name]; !ok {
			am[s.Name] = &collectorapi.ScrapeResult{}
		}
		am[s.Name].Items = append(am[s.Name].Items, s)
	}

	for k, v := range am {
		b, err := proto.Marshal(v)
		if err != nil {
			return err
		}

		cn := os.Getenv("CLUSTER_NAME")
		var filename string
		if cn != "" {
			filename = filepath.Join(
				c.SaveSamplesLocally.DirectoryPath,
				k, time.Now().Format(c.SaveSamplesLocally.TimeFormat)+"_"+cn+"_"+k+".scraperesult.pb")
		} else {
			filename = filepath.Join(
				c.SaveSamplesLocally.DirectoryPath,
				k, time.Now().Format(c.SaveSamplesLocally.TimeFormat)+"_"+k+".scraperesult.pb")
		}

		err = os.MkdirAll(filepath.Dir(filename), 0700)
		if err != nil {
			return err
		}

		err = os.WriteFile(filename, b, 0200)
		if err != nil {
			return err
		}
		err = os.Chmod(filename, 0600)
		if err != nil {
			return err
		}
	}

	return nil
}

type JSONLine struct {
	MetricName string            `json:"metricName"`
	Timestamp  time.Time         `json:"timestamp"`
	Value      float64           `json:"value"`
	Labels     map[string]string `json:"labels"`
}

func ProtoToJSON(s *collectorapi.SampleList, b *bytes.Buffer) error {
	// shard output files so they are more efficient to read
	for _, i := range s.Items {
		for _, v := range i.Values {
			for _, x := range v.Values {
				line := JSONLine{
					MetricName: s.MetricName,
					Timestamp:  s.Timestamp.AsTime(),
					Value:      x,
					Labels:     i.Labels,
				}
				line.Labels["data_source"] = s.Name
				line.Labels["cluster_name"] = s.ClusterName
				e := json.NewEncoder(b)
				e.SetIndent("", "")
				err := e.Encode(line)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *Collector) SaveScrapeResultToJSONFile(sr *collectorapi.ScrapeResult) error {
	if sr == nil {
		return nil
	}

	// shard output files so they are more efficient to read
	am := map[string]*bytes.Buffer{}
	for _, s := range sr.Items {
		if am[s.Name] == nil {
			am[s.Name] = &bytes.Buffer{}
		}
		if err := ProtoToJSON(s, am[s.Name]); err != nil {
			return err
		}
	}

	for k, v := range am {
		cn := os.Getenv("CLUSTER_NAME")
		var filename string
		if cn != "" {
			filename = filepath.Join(
				c.SaveSamplesLocally.DirectoryPath,
				k, time.Now().Format(c.SaveSamplesLocally.TimeFormat)+"_"+cn+"_"+k+"_metrics.json")
		} else {
			filename = filepath.Join(
				c.SaveSamplesLocally.DirectoryPath,
				k, time.Now().Format(c.SaveSamplesLocally.TimeFormat)+"_"+k+"_metrics.json")
		}

		err := os.MkdirAll(filepath.Dir(filename), 0700)
		if err != nil {
			return err
		}

		err = os.WriteFile(filename, v.Bytes(), 0200)
		if err != nil {
			return err
		}
		err = os.Chmod(filename, 0600)
		if err != nil {
			return err
		}
	}

	return nil
}
