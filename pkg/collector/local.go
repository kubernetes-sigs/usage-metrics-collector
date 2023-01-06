package collector

import (
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
	SourceType    string
	DirectoryPath string
	Save          bool
	TimeFormat    string
	SampleList    *collectorapi.SampleList
	C             *Collector
}

func (c *Collector) NewAggregatedSampleListBuilder(src string) *SampleListBuilder {
	return c.newSampleListBuilder(src, true)
}

func (c *Collector) NewSampleListBuilder(src string) *SampleListBuilder {
	return c.newSampleListBuilder(src, false)
}

// NewSampleListBuilder returns a SampleBuilder for
func (c *Collector) newSampleListBuilder(src string, createIfNoMatch bool) *SampleListBuilder {
	if c.SaveSamplesLocally == nil {
		return nil
	}
	ts := timestamppb.New(time.Now())
	sb := &SampleListBuilder{
		C:             c,
		SourceType:    src,
		DirectoryPath: c.SaveSamplesLocally.DirectoryPath,
		TimeFormat:    c.SaveSamplesLocally.TimeFormat,
		SampleList:    &collectorapi.SampleList{Timestamp: ts, Type: src},
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
func (sb *SampleListBuilder) NewSample(labels labelsValues) *collectorapi.Sample {
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
func (sb *SampleListBuilder) AddQuantityValues(s *collectorapi.Sample, resourceType string, source string, v ...resource.Quantity) {
	if sb == nil {
		return
	}

	m := &collectorapi.Metric{Source: source, ResourceType: resourceType}
	s.Values = append(s.Values, m)
	for i := range v {
		m.Values = append(m.Values, v[i].AsApproximateFloat64())
	}
}

// AddIntValues adds a Metric to the sample with the provided values
func (sb *SampleListBuilder) AddIntValues(s *collectorapi.Sample, resourceType string, source string, v ...int64) {
	if sb == nil {
		return
	}

	m := &collectorapi.Metric{Source: source, ResourceType: resourceType}
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
		filename = filepath.Join(sb.DirectoryPath, sb.SourceType, time.Now().Format(sb.TimeFormat)+"_"+cn+"_"+sb.SourceType+".samplelist.pb")
	} else {
		filename = filepath.Join(sb.DirectoryPath, sb.SourceType, time.Now().Format(sb.TimeFormat)+"_"+sb.SourceType+".samplelist.pb")
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

func (c *Collector) SaveScrapeResultToFile(sr *collectorapi.ScrapeResult) error {
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
