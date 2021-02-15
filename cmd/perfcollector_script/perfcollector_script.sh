#!/bin/bash

# // PCCollection is a raw measurement that is sunk into the network.
# type PCCollection struct {
#  Timestamp   time.Time     // Time of *overall* collection
#  Start       time.Time     // Start time of *this* collection
#  Duration    time.Duration // Time collection took
#  Frequency   time.Duration // Collection frequency
#  System      string        // System that was measured
#  Measurement string        // Raw measurement
# }

# type WrapPCCollection struct {
#  Site        uint64
#  Host        uint64
#  Run         uint64
#  Measurement *types.PCCollection
# }

# Wrap
SITE=1
HOST=0
RUN=0

# Collection
FREQUENCY=5000000000
WAIT=$(expr $FREQUENCY / 1000000000)

function measure {
	MEASUREMENT=$(cat $1)
	INNER=$(jq -n \
                  --arg ts "$TIMESTAMP" \
                  --argjson fr "$FREQUENCY" \
                  --arg sy "$1" \
                  --arg me "$MEASUREMENT" \
                  '{Timestamp: $ts, Frequency: $fr, System: $sy, Measurement: $me}')
	JSON_STRING=$(jq -n -r \
		--argjson site "$SITE" \
		--argjson host "$HOST" \
		--argjson run "$RUN" \
		--argjson measurement "$INNER" \
		'{Site: $site, Host: $host, Run: $run, Measurement: $measurement}'
	)
}

while :; do
	TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%S.%NZ")
	measure /proc/stat
	echo $JSON_STRING

	measure /proc/meminfo
	echo $JSON_STRING

	measure /proc/diskstats
	echo $JSON_STRING

	measure /proc/net/dev
	echo $JSON_STRING

	sleep $WAIT;
done
