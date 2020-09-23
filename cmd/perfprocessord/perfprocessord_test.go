package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/businessperformancetuning/perfcollector/cmd/perfprocessord/journal"
)

var j = []byte(`
{"Site":0,"Host":0,"Run":0,"Measurement":{"Timestamp":"2020-09-15T18:11:42.768224195-05:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/eno1/duplex","Measurement":"full"}}
{"Site":0,"Host":0,"Run":0,"Measurement":{"Timestamp":"2020-09-15T18:11:42.768224195-05:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/eno1/speed","Measurement":"1000"}}
{"Site":0,"Host":0,"Run":0,"Measurement":{"Timestamp":"2020-09-15T18:11:42.768224195-05:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/lo/duplex","Measurement":""}}
{"Site":0,"Host":0,"Run":0,"Measurement":{"Timestamp":"2020-09-15T18:11:42.768224195-05:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/lo/speed","Measurement":"0"}}
{"Site":0,"Host":1,"Run":0,"Measurement":{"Timestamp":"2020-09-15T18:11:42.770006364-05:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/eno1/duplex","Measurement":"full"}}
{"Site":0,"Host":1,"Run":0,"Measurement":{"Timestamp":"2020-09-15T18:11:42.770006364-05:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/eno1/speed","Measurement":"1000"}}
{"Site":0,"Host":1,"Run":0,"Measurement":{"Timestamp":"2020-09-15T18:11:42.770006364-05:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/lo/duplex","Measurement":""}}
{"Site":0,"Host":1,"Run":0,"Measurement":{"Timestamp":"2020-09-15T18:11:42.770006364-05:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/lo/speed","Measurement":"0"}}
`)

func TestJSONEncode(t *testing.T) {
	//r := bytes.NewReader(j)

	r, err := os.Open("/home/marco/gopath/src/github.com/businessperformancetuning/perfcollector/netcache.json")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	d := json.NewDecoder(r)
	entry := 0
	for {
		var wc journal.WrapPCCollection
		err := d.Decode(&wc)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		k := fmt.Sprintf("%v_%v_%v_%v", wc.Site, wc.Host, wc.Run,
			wc.Measurement.System)
		//cache[k] = wc.Measurement.Measurement
		entry++
		t.Logf("%v\n", k)
	}

}
