/*
 * Copyright (C) 2015 Red Hat, Inc.
 *
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 *
 */

package elasticsearch

import (
	"encoding/json"
	"errors"

	"github.com/lebauce/elastigo/lib"
	"github.com/mitchellh/mapstructure"
	"github.com/skydive-project/skydive/common"
	"github.com/skydive-project/skydive/filters"
	"github.com/skydive-project/skydive/flow"
	"github.com/skydive-project/skydive/logging"
	esclient "github.com/skydive-project/skydive/storage/elasticsearch"
)

const flowMapping = `
{
	"dynamic_templates": [
		{
			"strings": {
				"match": "*",
				"match_mapping_type": "string",
				"mapping": {
					"type": "string", "index": "not_analyzed", "doc_values": false
				}
			}
		},
		{
			"packets": {
				"match": "*Packets",
				"mapping": {
					"type": "long"
				}
			}
		},
		{
			"bytes": {
				"match": "*Bytes",
				"mapping": {
					"type": "long"
				}
			}
		},
		{
			"start": {
				"match": "Start",
				"mapping": {
					"type": "date", "format": "epoch_millis"
				}
			}
		},
		{
			"last": {
				"match": "Last",
				"mapping": {
					"type": "date", "format": "epoch_millis"
				}
			}
		}
	]
}`

const metricMapping = `
{
	"_parent": {
		"type": "flow"
	},
	"dynamic_templates": [
		{
			"packets": {
				"match": "*Packets",
				"mapping": {
					"type": "long"
				}
			}
		},
		{
			"bytes": {
				"match": "*Bytes",
				"mapping": {
					"type": "long"
				}
			}
		},
		{
			"start": {
				"match": "Start",
				"mapping": {
					"type": "date", "format": "epoch_millis"
				}
			}
		},
		{
			"last": {
				"match": "Last",
				"mapping": {
					"type": "date", "format": "epoch_millis"
				}
			}
		}
	]
}`

type ElasticSearchStorage struct {
	client *esclient.ElasticSearchClient
}

func (c *ElasticSearchStorage) StoreFlows(flows []*flow.Flow) error {
	if !c.client.Started() {
		return errors.New("ElasticSearchStorage is not yet started")
	}

	for _, f := range flows {
		if err := c.client.Index("flow", f.UUID, f); err != nil {
			logging.GetLogger().Errorf("Error while indexing: %s", err.Error())
			continue
		}

		if f.LastUpdateStart != 0 {
			// TODO submit a pull request to add bulk request with parent supported
			metric := map[string]interface{}{
				"ABBytes":   f.LastUpdateMetric.ABBytes,
				"BABytes":   f.LastUpdateMetric.BABytes,
				"ABPackets": f.LastUpdateMetric.ABPackets,
				"BAPackets": f.LastUpdateMetric.BAPackets,
				"Start":     f.LastUpdateStart,
				"Last":      f.LastUpdateLast,
			}
			if err := c.client.IndexChild("metric", f.UUID, "", metric); err != nil {
				logging.GetLogger().Errorf("Error while indexing: %s", err.Error())
				continue
			}
		}
	}

	return nil
}

func (c *ElasticSearchStorage) requestFromQuery(fsq filters.SearchQuery) (map[string]interface{}, error) {
	request := map[string]interface{}{"size": 10000}

	if fsq.PaginationRange != nil {
		if fsq.PaginationRange.To < fsq.PaginationRange.From {
			return request, errors.New("Incorrect PaginationRange, To < From")
		}

		request["from"] = fsq.PaginationRange.From
		request["size"] = fsq.PaginationRange.To - fsq.PaginationRange.From
	}

	return request, nil
}

func (c *ElasticSearchStorage) sendRequest(docType string, request map[string]interface{}) (elastigo.SearchResult, error) {
	q, err := json.Marshal(request)
	if err != nil {
		return elastigo.SearchResult{}, err
	}
	return c.client.Search(docType, string(q))
}

func (c *ElasticSearchStorage) SearchMetrics(fsq filters.SearchQuery, metricFilter *filters.Filter) (map[string][]*common.TimedMetric, error) {
	if !c.client.Started() {
		return nil, errors.New("ElasticSearchStorage is not yet started")
	}

	request, err := c.requestFromQuery(fsq)
	if err != nil {
		return nil, err
	}

	// do not escape flow as ES use sub object in that case
	flowQuery := c.client.FormatFilter(fsq.Filter, "", false)
	musts := []map[string]interface{}{{
		"has_parent": map[string]interface{}{
			"type":  "flow",
			"query": flowQuery,
		},
	}}

	metricQuery := c.client.FormatFilter(metricFilter, "", false)
	musts = append(musts, metricQuery)

	request["query"] = map[string]interface{}{
		"bool": map[string]interface{}{
			"must": musts,
		},
	}

	if fsq.Sort {
		request["sort"] = map[string]interface{}{
			fsq.SortBy: map[string]string{
				"order": fsq.SortOrder,
			},
		}
	}

	out, err := c.sendRequest("metric", request)
	if err != nil {
		return nil, err
	}

	metrics := map[string][]*common.TimedMetric{}
	if out.Hits.Len() > 0 {
		for _, d := range out.Hits.Hits {
			tm := new(common.TimedMetric)
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(*d.Source), &obj); err != nil {
				return nil, err
			}

			m := new(flow.FlowMetric)
			if err := mapstructure.Decode(obj, m); err != nil {
				return nil, err
			}

			tm.Start = int64(obj["Start"].(float64))
			tm.Last = int64(obj["Last"].(float64))
			tm.Metric = m
			metrics[d.Parent] = append(metrics[d.Parent], tm)
		}
	}

	return metrics, nil
}

func (c *ElasticSearchStorage) SearchFlows(fsq filters.SearchQuery) (*flow.FlowSet, error) {
	if !c.client.Started() {
		return nil, errors.New("ElasticSearchStorage is not yet started")
	}

	request, err := c.requestFromQuery(fsq)
	if err != nil {
		return nil, err
	}

	var query map[string]interface{}
	if fsq.Filter != nil {
		query = c.client.FormatFilter(fsq.Filter, "", false)
	}

	request["query"] = query

	if fsq.Sort {
		request["sort"] = map[string]interface{}{
			fsq.SortBy: map[string]string{
				"order": fsq.SortOrder,
			},
		}
	}

	out, err := c.sendRequest("flow", request)
	if err != nil {
		return nil, err
	}

	flowset := flow.NewFlowSet()
	if out.Hits.Len() > 0 {
		for _, d := range out.Hits.Hits {
			f := new(flow.Flow)
			if err := json.Unmarshal([]byte(*d.Source), f); err != nil {
				return nil, err
			}
			flowset.Flows = append(flowset.Flows, f)
		}
	}

	if fsq.Dedup {
		if err := flowset.Dedup(fsq.DedupBy); err != nil {
			return nil, err
		}
	}

	return flowset, nil
}

func (c *ElasticSearchStorage) Start() {
	go c.client.Start([]map[string][]byte{
		{"metric": []byte(metricMapping)},
		{"flow": []byte(flowMapping)}},
	)
}

func (c *ElasticSearchStorage) Stop() {
	c.client.Stop()
}

func New() (*ElasticSearchStorage, error) {
	client, err := esclient.NewElasticSearchClientFromConfig()
	if err != nil {
		return nil, err
	}

	return &ElasticSearchStorage{client: client}, nil
}
