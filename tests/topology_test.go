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

package tests

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/skydive-project/skydive/common"
	"github.com/skydive-project/skydive/tests/helper"
)

func TestBridgeOVS(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"ovs-vsctl add-br br-test1", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ovs-vsctl del-br br-test1", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			gremlin := "g"
			if !c.time.IsZero() {
				gremlin += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			gremlin += `.V().Has("Type", "ovsbridge", "Name", "br-test1")`
			gremlin += `.Out("Type", "ovsport", "Name", "br-test1")`
			gremlin += `.Out("Type", "internal", "Name", "br-test1", "Driver", "openvswitch")`

			// we have 2 links between ovsbridge and ovsport, this
			// results in 2 out nodes which are the same node so we Dedup
			gremlin += ".Dedup()"

			nodes, err := gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestPatchOVS(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"ovs-vsctl add-br br-test1", true},
			{"ovs-vsctl add-br br-test2", true},
			{"ovs-vsctl add-port br-test1 patch-br-test2 -- set interface patch-br-test2 type=patch", true},
			{"ovs-vsctl add-port br-test2 patch-br-test1 -- set interface patch-br-test1 type=patch", true},
			{"ovs-vsctl set interface patch-br-test2 option:peer=patch-br-test1", true},
			{"ovs-vsctl set interface patch-br-test1 option:peer=patch-br-test2", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ovs-vsctl del-br br-test1", true},
			{"ovs-vsctl del-br br-test2", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			gremlin := "g"
			if !c.time.IsZero() {
				gremlin += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			gremlin += `.V().Has("Type", "patch", "Name", "patch-br-test1", "Driver", "openvswitch")`
			gremlin += `.Both("Type", "patch", "Name", "patch-br-test2", "Driver", "openvswitch")`

			nodes, err := gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestInterfaceOVS(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"ovs-vsctl add-br br-test1", true},
			{"ovs-vsctl add-port br-test1 intf1 -- set interface intf1 type=internal", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ovs-vsctl del-br br-test1", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			prefix := "g"
			if !c.time.IsZero() {
				prefix += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			gremlin := prefix + `.V().Has("Type", "internal", "Name", "intf1", "Driver", "openvswitch").HasKey("UUID").HasKey("MAC")`
			nodes, err := gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected one 'intf1' node with MAC and UUID attributes, got %+v", nodes)
			}

			gremlin = prefix + `.V().Has("Name", "intf1", "Type", Ne("ovsport"))`
			nodes, err = gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected one 'intf1' node with type different than 'ovsport', got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestVeth(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"ip l add vm1-veth0 type veth peer name vm1-veth1", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ip link del vm1-veth0", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			prefix := "g"
			if !c.time.IsZero() {
				prefix += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			nodes, err := gh.GetNodes(prefix + `.V().Has("Type", "veth", "Name", "vm1-veth0").Both("Type", "veth", "Name", "vm1-veth1")`)
			if err != nil {
				return err
			}
			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node, got %+v", nodes)
			}
			return nil
		},
	}

	RunTest(t, test)
}

func TestBridge(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"brctl addbr br-test", true},
			{"ip tuntap add mode tap dev intf1", true},
			{"brctl addif br-test intf1", true},
		},

		tearDownCmds: []helper.Cmd{
			{"brctl delbr br-test", true},
			{"ip link del intf1", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			prefix := "g"
			if !c.time.IsZero() {
				prefix += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			nodes, err := gh.GetNodes(prefix + `.V().Has("Type", "bridge", "Name", "br-test").Out("Name", "intf1")`)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestMacNameUpdate(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"ip l add vm1-veth0 type veth peer name vm1-veth1", true},
			{"ip l set vm1-veth1 name vm1-veth2", true},
			{"ip l set vm1-veth2 address 00:00:00:00:00:aa", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ip link del vm1-veth0", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh

			prefix := "g"
			if !c.time.IsZero() {
				prefix += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			newNodes, err := gh.GetNodes(prefix + `.V().Has("Name", "vm1-veth2", "MAC", "00:00:00:00:00:aa")`)
			if err != nil {
				return err
			}

			oldNodes, err := gh.GetNodes(prefix + `.V().Has("Name", "vm1-veth1")`)
			if err != nil {
				return err
			}

			if len(newNodes) != 1 || len(oldNodes) != 0 {
				return fmt.Errorf("Expected one name named vm1-veth2 and zero named vm1-veth1")
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestNameSpace(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"ip netns add ns1", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ip netns del ns1", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh

			prefix := "g"
			if !c.time.IsZero() {
				prefix += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			nodes, err := gh.GetNodes(prefix + `.V().Has("Name", "ns1", "Type", "netns")`)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestNameSpaceVeth(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"ip netns add ns1", true},
			{"ip l add vm1-veth0 type veth peer name vm1-veth1 netns ns1", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ip link del vm1-veth0", true},
			{"ip netns del ns1", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			prefix := "g"
			if !c.time.IsZero() {
				prefix += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			nodes, err := gh.GetNodes(prefix + `.V().Has("Name", "ns1", "Type", "netns").Out("Name", "vm1-veth1", "Type", "veth")`)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestNameSpaceOVSInterface(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"ip netns add ns1", true},
			{"ovs-vsctl add-br br-test1", true},
			{"ovs-vsctl add-port br-test1 intf1 -- set interface intf1 type=internal", true},
			{"ip l set intf1 netns ns1", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ovs-vsctl del-br br-test1", true},
			{"ip netns del ns1", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			prefix := "g"
			if !c.time.IsZero() {
				prefix += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			nodes, err := gh.GetNodes(prefix + `.V().Has("Name", "ns1", "Type", "netns").Out("Name", "intf1", "Type", "internal")`)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node of type internal, got %+v", nodes)
			}

			nodes, err = gh.GetNodes(prefix + `.V().Has("Name", "intf1", "Type", "internal")`)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestDockerSimple(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"docker run -d -t -i --name test-skydive-docker busybox", false},
		},

		tearDownCmds: []helper.Cmd{
			{"docker rm -f test-skydive-docker", false},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			gremlin := "g"
			if !c.time.IsZero() {
				gremlin += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			gremlin += `.V().Has("Name", "test-skydive-docker", "Type", "netns", "Manager", "docker")`
			gremlin += `.Out("Type", "container", "Docker/ContainerName", "/test-skydive-docker")`

			nodes, err := gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 node, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestDockerShareNamespace(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"docker run -d -t -i --name test-skydive-docker busybox", false},
			{"docker run -d -t -i --name test-skydive-docker2 --net=container:test-skydive-docker busybox", false},
		},

		tearDownCmds: []helper.Cmd{
			{"docker rm -f test-skydive-docker", false},
			{"docker rm -f test-skydive-docker2", false},
		},

		check: func(c *TestContext) error {
			gh := c.gh

			gremlin := "g"
			if !c.time.IsZero() {
				gremlin += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			gremlin += `.V().Has("Type", "netns", "Manager", "docker")`
			nodes, err := gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			switch len(nodes) {
			case 0:
				return errors.New("No namespace found")
			case 1:
				gremlin += `.Out().Has("Type", "container", "Docker/ContainerName", Within("/test-skydive-docker", "/test-skydive-docker2"))`

				nodes, err = gh.GetNodes(gremlin)
				if err != nil {
					return err
				}

				if len(nodes) != 2 {
					return fmt.Errorf("Expected 2 nodes, got %+v", nodes)
				}
			default:
				return fmt.Errorf("There should be only one namespace managed by Docker, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestDockerNetHost(t *testing.T) {
	test := &Test{
		setupCmds: []helper.Cmd{
			{"docker run -d -t -i --net=host --name test-skydive-docker busybox", false},
		},

		tearDownCmds: []helper.Cmd{
			{"docker rm -f test-skydive-docker", false},
		},

		check: func(c *TestContext) error {
			gh := c.gh

			prefix := "g"
			if !c.time.IsZero() {
				prefix += fmt.Sprintf(".Context(%d)", common.UnixMillis(c.time))
			}

			gremlin := prefix + `.V().Has("Docker/ContainerName", "/test-skydive-docker", "Type", "container")`
			nodes, err := gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			if len(nodes) != 1 {
				return fmt.Errorf("Expected 1 container, got %+v", nodes)
			}

			gremlin = prefix + `.V().Has("Type", "netns", "Manager", "docker", "Name", "test-skydive-docker")`
			nodes, err = gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			if len(nodes) != 0 {
				return fmt.Errorf("There should be only no namespace managed by Docker, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestInterfaceUpdate(t *testing.T) {
	start := time.Now()

	test := &Test{
		mode: OneShot,
		setupCmds: []helper.Cmd{
			{"ip netns add iu", true},
			{"sleep 1", false},
			{"ip netns exec iu ip link set lo up", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ip netns del iu", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh

			now := time.Now()
			gremlin := fmt.Sprintf("g.Context(%d, %d)", common.UnixMillis(now), int(now.Sub(start).Seconds()))
			gremlin += `.V().Has("Name", "iu", "Type", "netns").Out().Has("Name", "lo")`

			nodes, err := gh.GetNodes(gremlin)
			if err != nil {
				return err
			}

			if len(nodes) < 2 {
				return fmt.Errorf("Expected at least 2 nodes, got %+v", nodes)
			}

			hasDown := false
			hasUp := false
			for i := range nodes {
				if !hasDown && nodes[i].Metadata()["State"].(string) == "DOWN" {
					hasDown = true
				}
				if !hasUp && nodes[i].Metadata()["State"].(string) == "UP" {
					hasUp = true
				}
			}

			if !hasUp || !hasDown {
				return fmt.Errorf("Expected one node up and one node down, got %+v", nodes)
			}

			return nil
		},
	}

	RunTest(t, test)
}

func TestInterfaceMetrics(t *testing.T) {
	start := time.Now()

	test := &Test{
		setupCmds: []helper.Cmd{
			{"ip netns add im", true},
			{"sleep 1", false},
			{"ip netns exec im ip link set lo up", true},
			{"ip netns exec im ping -c 3 127.0.0.1", true},
		},

		tearDownCmds: []helper.Cmd{
			{"ip netns del im", true},
		},

		check: func(c *TestContext) error {
			gh := c.gh
			gremlin := "g"
			if !c.time.IsZero() {
				gremlin += fmt.Sprintf(".Context(%d, %d)", common.UnixMillis(c.time), int(c.time.Sub(start).Seconds()+5))
			}

			gremlin += `.V().Has("Name", "im", "Type", "netns").Out().Has("Name", "lo").Metrics().Aggregates()`
			metrics, err := gh.GetInterfaceAggregatedMetrics(gremlin)
			if err != nil {
				return err
			}

			if len(metrics) != 1 {
				return fmt.Errorf("Expected one aggregated metric, got %+v", metrics)
			}

			if metrics[0].TxPackets != 6 {
				return fmt.Errorf("Expected 6 TxPackets, got %d", metrics[0].TxPackets)
			}

			return nil
		},
	}

	RunTest(t, test)
}
