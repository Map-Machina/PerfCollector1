# perfcpumeasure

Performance CPU measuring and training tool.

This tool measures the total amount of units this host can run and then dumps a
percentile table to `stdout`. The training data is required by the `perfreplay`
tool.

## Usage
```
Usage of perfcpumeasure:
  perfreplay [flags]
Flags:
  -C value
        config file
  -v    verbose
        Output is sent to stderr.
  -V    Show version and exit
  --siteid unsigned integer
        Numerical site id, e.g. 1
  --host unsigned integer
        Host ID that is being measured.
```

## Examples

Dump training data to `stdout`:
```
$ perfcpumeasure --siteid=1 --host=0
{"siteid":1,"host":0,"busy":10,"units":9}
{"siteid":1,"host":0,"busy":20,"units":17}
{"siteid":1,"host":0,"busy":30,"units":26}
{"siteid":1,"host":0,"busy":40,"units":34}
{"siteid":1,"host":0,"busy":50,"units":43}
{"siteid":1,"host":0,"busy":60,"units":52}
{"siteid":1,"host":0,"busy":70,"units":60}
{"siteid":1,"host":0,"busy":80,"units":69}
{"siteid":1,"host":0,"busy":90,"units":77}
{"siteid":1,"host":0,"busy":100,"units":85}
```

Dump training data to `training.json` and print training progress:
```
$ perfcpumeasure --siteid=1 --host=0 -v > training.json
=== looking for 10 busy 10.6 (load 10) units 9
=== looking for 20 busy 19.9 (load 20) units 17
=== looking for 30 busy 30.4 (load 30) units 26
=== looking for 40 busy 41.0 (load 40) units 35
=== looking for 50 busy 51.5 (load 50) units 44
=== looking for 60 busy 60.9 (load 60) units 52
=== looking for 70 busy 71.5 (load 70) units 61
=== looking for 80 busy 81.5 (load 80) units 70
=== looking for 90 busy 92.6 (load 90) -- busy 91.4 (load 89) units 78
```
