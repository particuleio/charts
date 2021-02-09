// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model_test

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/memory"
	"istio.io/istio/pilot/pkg/model"
	mock_config "istio.io/istio/pilot/test/mock"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/host"
	"istio.io/istio/pkg/config/labels"
	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/config/schema/collection"
	"istio.io/istio/pkg/config/schema/collections"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/resource"
)

// getByMessageName finds a schema by message name if it is available
// In test setup, we do not have more than one descriptor with the same message type, so this
// function is ok for testing purpose.
func getByMessageName(schemas collection.Schemas, name string) (collection.Schema, bool) {
	for _, s := range schemas.All() {
		if s.Resource().Proto() == name {
			return s, true
		}
	}
	return nil, false
}

func schemaFor(kind, proto string) collection.Schema {
	return collection.Builder{
		Name: kind,
		Resource: resource.Builder{
			Kind:   kind,
			Plural: kind + "s",
			Proto:  proto,
		}.BuildNoValidate(),
	}.MustBuild()
}

func TestConfigDescriptor(t *testing.T) {
	a := schemaFor("a", "proxy.A")
	schemas := collection.SchemasFor(
		a,
		schemaFor("b", "proxy.B"),
		schemaFor("c", "proxy.C"))
	want := []string{"a", "b", "c"}
	got := schemas.Kinds()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("descriptor.Types() => got %+vwant %+v", spew.Sdump(got), spew.Sdump(want))
	}

	aType, aExists := schemas.FindByGroupVersionKind(a.Resource().GroupVersionKind())
	if !aExists || !reflect.DeepEqual(aType, a) {
		t.Errorf("descriptor.GetByType(a) => got %+v, want %+v", aType, a)
	}
	if _, exists := schemas.FindByGroupVersionKind(config.GroupVersionKind{Kind: "missing"}); exists {
		t.Error("descriptor.GetByType(missing) => got true, want false")
	}

	aSchema, aSchemaExists := getByMessageName(schemas, a.Resource().Proto())
	if !aSchemaExists || !reflect.DeepEqual(aSchema, a) {
		t.Errorf("descriptor.GetByMessageName(a) => got %+v, want %+v", aType, a)
	}
	_, aSchemaNotExist := getByMessageName(schemas, "blah")
	if aSchemaNotExist {
		t.Errorf("descriptor.GetByMessageName(blah) => got true, want false")
	}
}

func TestEventString(t *testing.T) {
	cases := []struct {
		in   model.Event
		want string
	}{
		{model.EventAdd, "add"},
		{model.EventUpdate, "update"},
		{model.EventDelete, "delete"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("Failed: got %q want %q", got, c.want)
		}
	}
}

func TestPortList(t *testing.T) {
	pl := model.PortList{
		{Name: "http", Port: 80, Protocol: protocol.HTTP},
		{Name: "http-alt", Port: 8080, Protocol: protocol.HTTP},
	}

	gotNames := pl.GetNames()
	wantNames := []string{"http", "http-alt"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Errorf("GetNames() failed: got %v want %v", gotNames, wantNames)
	}

	cases := []struct {
		name  string
		port  *model.Port
		found bool
	}{
		{name: pl[0].Name, port: pl[0], found: true},
		{name: "foobar", found: false},
	}

	for _, c := range cases {
		gotPort, gotFound := pl.Get(c.name)
		if c.found != gotFound || !reflect.DeepEqual(gotPort, c.port) {
			t.Errorf("Get() failed: gotFound=%v wantFound=%v\ngot %+vwant %+v",
				gotFound, c.found, spew.Sdump(gotPort), spew.Sdump(c.port))
		}
	}
}

func TestSubsetKey(t *testing.T) {
	hostname := host.Name("hostname")
	cases := []struct {
		hostname host.Name
		subset   string
		port     int
		want     string
	}{
		{
			hostname: "hostname",
			subset:   "subset",
			port:     80,
			want:     "outbound|80|subset|hostname",
		},
		{
			hostname: "hostname",
			subset:   "",
			port:     80,
			want:     "outbound|80||hostname",
		},
	}

	for _, c := range cases {
		got := model.BuildSubsetKey(model.TrafficDirectionOutbound, c.subset, hostname, c.port)
		if got != c.want {
			t.Errorf("Failed: got %q want %q", got, c.want)
		}

		// test parse subset key. ParseSubsetKey is the inverse of BuildSubsetKey
		_, s, h, p := model.ParseSubsetKey(got)
		if s != c.subset || h != c.hostname || p != c.port {
			t.Errorf("Failed: got %s,%s,%d want %s,%s,%d", s, h, p, c.subset, c.hostname, c.port)
		}
	}
}

func TestLabelsEquals(t *testing.T) {
	cases := []struct {
		a, b labels.Instance
		want bool
	}{
		{
			a: nil,
			b: labels.Instance{"a": "b"},
		},
		{
			a: labels.Instance{"a": "b"},
			b: nil,
		},
		{
			a:    labels.Instance{"a": "b"},
			b:    labels.Instance{"a": "b"},
			want: true,
		},
	}
	for _, c := range cases {
		if got := c.a.Equals(c.b); got != c.want {
			t.Errorf("Failed: got eq=%v want=%v for %q ?= %q", got, c.want, c.a, c.b)
		}
	}
}

func TestConfigKey(t *testing.T) {
	cfg := mock_config.Make("ns", 2)
	want := "MockConfig/ns/mock-config2"
	if key := cfg.Meta.Key(); key != want {
		t.Fatalf("config.Key() => got %q, want %q", key, want)
	}
}

func TestResolveShortnameToFQDN(t *testing.T) {
	tests := []struct {
		name string
		meta config.Meta
		out  host.Name
	}{
		{
			"*", config.Meta{}, "*",
		},
		{
			"*", config.Meta{Namespace: "default", Domain: "cluster.local"}, "*",
		},
		{
			"foo", config.Meta{Namespace: "default", Domain: "cluster.local"}, "foo.default.svc.cluster.local",
		},
		{
			"foo.bar", config.Meta{Namespace: "default", Domain: "cluster.local"}, "foo.bar",
		},
		{
			"foo", config.Meta{Domain: "cluster.local"}, "foo.svc.cluster.local",
		},
		{
			"foo", config.Meta{Namespace: "default"}, "foo.default",
		},
		{
			"42.185.131.210", config.Meta{Namespace: "default"}, "42.185.131.210",
		},
		{
			"42.185.131.210", config.Meta{Namespace: "cluster.local"}, "42.185.131.210",
		},
		{
			"2a00:4000::614", config.Meta{Namespace: "default"}, "2a00:4000::614",
		},
		{
			"2a00:4000::614", config.Meta{Namespace: "cluster.local"}, "2a00:4000::614",
		},
	}

	for idx, tt := range tests {
		t.Run(fmt.Sprintf("[%d] %s", idx, tt.out), func(t *testing.T) {
			if actual := model.ResolveShortnameToFQDN(tt.name, tt.meta); actual != tt.out {
				t.Fatalf("model.ResolveShortnameToFQDN(%q, %v) = %q wanted %q", tt.name, tt.meta, actual, tt.out)
			}
		})
	}
}

func TestMostSpecificHostMatch(t *testing.T) {
	tests := []struct {
		in     []host.Name
		needle host.Name
		want   host.Name
	}{
		// this has to be a sorted list
		{[]host.Name{}, "*", ""},
		{[]host.Name{"*.foo.com", "*.com"}, "bar.foo.com", "*.foo.com"},
		{[]host.Name{"*.foo.com", "*.com"}, "foo.com", "*.com"},
		{[]host.Name{"foo.com", "*.com"}, "*.foo.com", "*.com"},

		{[]host.Name{"*.foo.com", "foo.com"}, "foo.com", "foo.com"},
		{[]host.Name{"*.foo.com", "foo.com"}, "*.foo.com", "*.foo.com"},

		// this passes because we sort alphabetically
		{[]host.Name{"bar.com", "foo.com"}, "*.com", ""},

		{[]host.Name{"bar.com", "*.foo.com"}, "*foo.com", ""},
		{[]host.Name{"foo.com", "*.foo.com"}, "*foo.com", ""},

		// should prioritize closest match
		{[]host.Name{"*.bar.com", "foo.bar.com"}, "foo.bar.com", "foo.bar.com"},
		{[]host.Name{"*.foo.bar.com", "bar.foo.bar.com"}, "bar.foo.bar.com", "bar.foo.bar.com"},

		// should not match non-wildcards for wildcard needle
		{[]host.Name{"bar.foo.com", "foo.bar.com"}, "*.foo.com", ""},
		{[]host.Name{"foo.bar.foo.com", "bar.foo.bar.com"}, "*.bar.foo.com", ""},
	}

	for idx, tt := range tests {
		m := make(map[host.Name]struct{})
		for _, h := range tt.in {
			m[h] = struct{}{}
		}

		t.Run(fmt.Sprintf("[%d] %s", idx, tt.needle), func(t *testing.T) {
			actual, found := model.MostSpecificHostMatch(tt.needle, m, tt.in)
			if tt.want != "" && !found {
				t.Fatalf("model.MostSpecificHostMatch(%q, %v) = %v, %t; want: %v", tt.needle, tt.in, actual, found, tt.want)
			} else if actual != tt.want {
				t.Fatalf("model.MostSpecificHostMatch(%q, %v) = %v, %t; want: %v", tt.needle, tt.in, actual, found, tt.want)
			}
		})
	}
}

func BenchmarkMostSpecificHostMatch(b *testing.B) {
	benchmarks := []struct {
		name     string
		needle   host.Name
		baseHost string
		hosts    []host.Name
		hostsMap map[host.Name]struct{}
		time     int
	}{
		{"10Exact", host.Name("foo.bar.com.10"), "foo.bar.com", []host.Name{}, nil, 10},
		{"50Exact", host.Name("foo.bar.com.50"), "foo.bar.com", []host.Name{}, nil, 50},
		{"100Exact", host.Name("foo.bar.com.100"), "foo.bar.com", []host.Name{}, nil, 100},
		{"1000Exact", host.Name("foo.bar.com.1000"), "foo.bar.com", []host.Name{}, nil, 1000},
		{"5000Exact", host.Name("foo.bar.com.5000"), "foo.bar.com", []host.Name{}, nil, 5000},

		{"10DestRuleWildcard", host.Name("foo.bar.com.10"), "*.foo.bar.com", []host.Name{}, nil, 10},
		{"50DestRuleWildcard", host.Name("foo.bar.com.50"), "*.foo.bar.com", []host.Name{}, nil, 50},
		{"100DestRuleWildcard", host.Name("foo.bar.com.100"), "*.foo.bar.com", []host.Name{}, nil, 100},
		{"1000DestRuleWildcard", host.Name("foo.bar.com.1000"), "*.foo.bar.com", []host.Name{}, nil, 1000},
		{"5000DestRuleWildcard", host.Name("foo.bar.com.5000"), "*.foo.bar.com", []host.Name{}, nil, 5000},

		{"10NeedleWildcard", host.Name("*.bar.foo.bar.com"), "*.foo.bar.com", []host.Name{}, nil, 10},
		{"50NeedleWildcard", host.Name("*.bar.foo.bar.com"), "*.foo.bar.com", []host.Name{}, nil, 50},
		{"100NeedleWildcard", host.Name("*.bar.foo.bar.com"), "*.foo.bar.com", []host.Name{}, nil, 100},
		{"1000NeedleWildcard", host.Name("*.bar.foo.bar.com"), "*.foo.bar.com", []host.Name{}, nil, 1000},
		{"5000NeedleWildcard", host.Name("*.bar.foo.bar.com"), "*.foo.bar.com", []host.Name{}, nil, 5000},
	}

	for _, bm := range benchmarks {
		bm.hostsMap = make(map[host.Name]struct{}, bm.time)

		for i := 1; i <= bm.time; i++ {
			h := host.Name(bm.baseHost + "." + strconv.Itoa(i))
			bm.hosts = append(bm.hosts, h)
			bm.hostsMap[h] = struct{}{}
		}

		b.Run(bm.name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				_, _ = model.MostSpecificHostMatch(bm.needle, bm.hostsMap, bm.hosts)
			}
		})
	}
}

func TestAuthorizationPolicies(t *testing.T) {
	store := model.MakeIstioStore(memory.Make(collections.Pilot))
	tests := []struct {
		namespace  string
		expectName map[string]bool
	}{
		{namespace: "wrong", expectName: nil},
		{namespace: "default", expectName: map[string]bool{"policy2": true}},
		{namespace: "istio-system", expectName: map[string]bool{"policy1": true, "policy3": true}},
	}

	for _, tt := range tests {
		cfg := store.AuthorizationPolicies(tt.namespace)
		if tt.expectName != nil {
			for _, cfg := range cfg {
				if !tt.expectName[cfg.Name] {
					t.Errorf("model.AuthorizationPolicy: expecting %v, but got %v", tt.expectName, cfg)
				}
			}
		} else if len(cfg) != 0 {
			t.Errorf("model.AuthorizationPolicy: expecting nil, but got %v", cfg)
		}
	}
}

type fakeStore struct {
	model.ConfigStore
	cfg map[config.GroupVersionKind][]config.Config
	err error
}

func (l *fakeStore) List(typ config.GroupVersionKind, namespace string) ([]config.Config, error) {
	ret := l.cfg[typ]
	return ret, l.err
}

func (l *fakeStore) Schemas() collection.Schemas {
	return collections.Pilot
}

func TestIstioConfigStore_ServiceEntries(t *testing.T) {
	ns := "ns1"
	l := &fakeStore{
		cfg: map[config.GroupVersionKind][]config.Config{
			gvk.ServiceEntry: {
				{
					Meta: config.Meta{
						Name:      "request-count-1",
						Namespace: ns,
					},
					Spec: &networking.ServiceEntry{
						Hosts: []string{"*.googleapis.com"},
						Ports: []*networking.Port{
							{
								Name:     "https",
								Number:   443,
								Protocol: "HTTP",
							},
						},
					},
				},
			},
		},
	}
	ii := model.MakeIstioStore(l)
	cfgs := ii.ServiceEntries()

	if len(cfgs) != 1 {
		t.Fatalf("did not find 1 matched ServiceEntry, \n%v", cfgs)
	}
}

func TestIstioConfigStore_Gateway(t *testing.T) {
	workloadLabels := labels.Collection{}
	now := time.Now()
	gw1 := config.Config{
		Meta: config.Meta{
			Name:              "name1",
			Namespace:         "zzz",
			CreationTimestamp: now,
		},
		Spec: &networking.Gateway{},
	}
	gw2 := config.Config{
		Meta: config.Meta{
			Name:              "name1",
			Namespace:         "aaa",
			CreationTimestamp: now,
		},
		Spec: &networking.Gateway{},
	}
	gw3 := config.Config{
		Meta: config.Meta{
			Name:              "name1",
			Namespace:         "ns2",
			CreationTimestamp: now.Add(time.Second * -1),
		},
		Spec: &networking.Gateway{},
	}

	l := &fakeStore{
		cfg: map[config.GroupVersionKind][]config.Config{
			gvk.Gateway: {gw1, gw2, gw3},
		},
	}
	ii := model.MakeIstioStore(l)

	// Gateways should be returned in a stable order
	expectedConfig := []config.Config{
		gw3, // first config by timestamp
		gw2, // timestamp match with gw1, but name comes first
		gw1, // timestamp match with gw2, but name comes last
	}
	cfgs := ii.Gateways(workloadLabels)

	if !reflect.DeepEqual(expectedConfig, cfgs) {
		t.Errorf("Got different Config, Excepted:\n%v\n, Got: \n%v\n", expectedConfig, cfgs)
	}
}
