# Performance monitoring and replay tools

These tools are at this time strictly Linux only. Substantial effort must be
made to convert them over to other OS'.

Do read through the entire document first before attempting to set up a
collection. The setup at this time requires some jumping around and having read
it in its entirety before attempting an install makes it easier to follow.

The theory of this tool set is to collect and parse performance data. Once data
has been collected it can be used for visualization and to replay the machine
load on a differently shaped machine.

Install `perfcollectord` on the machines that need to be measured. This tool
has a builtin `ssh` daemon that listens for connections from a single
`perfprocessord`. The relationship is 1:N (`perfprocessord`:`perfcollectord`).

The data that is captured from all systems is aggregated into a single journal
file. Typically `~/.perfprocessord/data/journal` This file is encrypted with a
key that is generated from the `siteid`, `sitename`, `mac address` and
`license`. The point of this encryption is that when installed on a customer
site it can't be used by the customer. The mac address MUST exist on the
machine that runs `perfprocessord`. All this information is case sensitive and
must exactly match throughout usage in the tool set. Failure to do this will
result in files that cannot be decrypt and processed. Before starting a large
collection one MUST verify that the journal can be indeed decrypted.

The `perflicense` tool is used to generate licenses for `perfprocessord`. The
information is also used to encrypt journal files. Care must be taken that the
exact same information is used when transcribing the license into the various
tools.

The `perfjournal` tool is used to decrypt the encrypted journal. It outputs
either a CSV or a JSON file with the cubed performance data.

The `perfreplay` tool is used to replay the load of a selected machine that was
capture in a journal file. This tool does require a raw performance measurement
that must be run on the source system (the one that runs `perfcollectord`); the
tool that measures raw performance is `perfcpumeasure`; it stresses the system
for about 15 seconds while trying to determine raw CPU usage for every 10th
percentile. The replay is one to one meaning it will run exactly at the same
speed as it was collected.

## Directory layout

The repository contains individual tools, libraries and scripts.

All tools live in the `cmd` directory.
* `cmd/perfcollectord` - Performance data collector daemon.
* `cmd/perfcpumeasure` - Tool to generate a CPU performance profile.
* `cmd/perfjournal` - Tool to decrypt collected performance data.
* `cmd/perflicense` - Tool to generate a time restrained license.
* `cmd/perfload` - Generic tool to generate load. This is a development tool and is not built by the release script.
* `cmd/perfprocessord` - Performance data aggregator and cruncher daemon.
* `cmd/perfreplay` - Performance data replay tool.
* `cmd/skeleton` - Skeleton app, do not use. This is not built by the release script.

Other directories
* `channel` - Library for passing generic data through channels (here be dragons)
* `database` - Library to interact with SQL databases (currently incomplete/broken)
* `load` - Library that is used for load generation.
* `parser` - Library that converts raw `/proc` and `/sys` to database and `sar` format.
* `rpmbuild` - Incomplete scripts to build and rpm to install things as a service.
* `scripts` - Scripts to build a release of the suite.
* `sizer` - Test code, do not use.
* `types` - Library that defines API between collector and processor.
* `util` - Library with often used functions.

## Installing go

This is OS specific. Most Linux distros have some sort of installer.

Here is an example to install it on RHEL8:
https://computingforgeeks.com/how-to-install-go-on-rhel-8/

## Compiling tools

This example expects `$GOPATH` to be set per the installation section.

Clone the repo:
```
$ mkdir -p $GOPATH/src/github.com/businessperformancetuning
$ cd $GOPATH/src/github.com/businessperformancetuning
$ git clone git@github.com:businessperformancetuning/perfcollector.git
```

Build a release:
```
$ cd perfcollector
$ ./scripts/release.sh
```

This creates a directory with 32 and 64 bit Linux binaries and a manifest with
digests. The tar file marked `i386` is 32 bit and the one marked `amd64` is 64
bit. The 32 bit version is built but should not be used unless necessary.

## Installation

Copy the tar file to the target systems and untar it. For example:
```
$ scp perfcollector-linux-amd64-20201206-01.tar.gz myprocessormachine:
$ ssh myprocessormachine
$ tar zxvf perfcollector-linux-amd64-20201206-01.tar.gz
$ cp perfcollector-linux-amd64-20201206-01/* ~/bin
```

This assumes that the target system has a `bin` directory in the home
directory. Repeat these steps for all machines that are being measured as well.

The other place where these tools need to end up are on the machines that going
to process the journal and where a journal is being replayed.

## perflicense

Create a new license to be used with `perfprocessord`, `perfjournal` and
`perfreplay`.

It is documented here: https://github.com/businessperformancetuning/perfcollector/tree/master/cmd/perflicense

## perfprocessord in sink mode

In practise, it is wise to install `perfcollectord` machines first since
`perfprocessord` will try to connect to them once launched. The reason this
guide shows this backwards is because of figuring out the ssh key fingerprint
while not jumping around in explaining what needs to be done on a single
machine.

Create an ssh key that will be used to login to the `perfcollectord` machines.
```
$ ssh-keygen -t ed25519
```

Determine the freshly generated key SHA256 fingerprint:
```
$ ssh-keygen -l -f ~/.ssh/id_ed25519 -t SHA256
256 SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE marco@void (ED25519)
```

Make note of the portion that is prefixed with `SHA256:`. In this case the
fingerprint that will be needed to be copy/pasted into the `perfcollectord`
configuration is: `SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE`.

Launch the `perfprocessord` daemon with two collection hosts as such:
```
$ perfprocessord --sshid=~/.ssh/id_ed25519 --hosts=1:0/127.0.0.1:2222 --hosts=1:1/10.170.0.5:2222 --journal=1 --siteid=1 --sitename='Evil Corp' --license=6f37-6910-b2a0-e858-9657-f08d
```

All command line switches can also be stored in a configuration file. The
format is always ~/.TOOLNAME/$TOOLNAME.conf. For example,
`.perfprocessord/perfprocessord.conf` Any lines prefixed with `#` are ignored.

Here is an example that has the exact same options as the command line example:
```
$ cat ~/.perfprocessord/perfprocessord.conf 
sshid=~/.ssh/id_ed25519

# Enable journal mode
journal=1

hosts=1:0/127.0.0.1:2222
hosts=1:1/10.170.0.5:2222

siteid=1
sitename=Evil Corp
license=6f37-6910-b2a0-e858-9657-f08d
```

Note that this example cannot be used verbatim because the license has a
time bomb builtin. The license must be generated every time for each
deployment. Ensure there is no trailing whitespace on the license information.

The `hosts` entries have the following format: `<site id>:<host id>/<ip
address>:<port>`. The `<site id>:<host id>` tuple must be unique. The site is
must be the same for all hosts. The hosts do not require consequtive
identification numbers.

The tool supports collecting raw data directly into a database but that support
is currently not functional and is therefore not documented.

## perfcollectord

Example of launching a `perfcollectord`:
```
$ perfcollectord --sshid=~/.ssh/id_ed25519 --listen=127.0.0.1:2222 --allowedkeys=SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE
```

Note that the `--allowedkeys` entry is identical to the fingerprint  that was
generated in the prior section.

The command line switches can be put into a configuration file as well. For
example:
```
$ cat ~/.perfcollectord/perfcollectord.conf
sshid=~/.ssh/id_ed25519
listen=127.0.0.1:2222
allowedkeys=SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE
```

The `--listen` flag is optional. If omitted the daemon will listen on all IP
addresses on port `2222`.

## perfprocessord single shot commands

The `perfprocessord` tool also has single shot commands. Those are meant to
start, stop and monitor collections.

* `start` start performance data collection.
* `stop` stop performance data collection.
* `status` returns collector status.
* `once` returns a single `/proc` or `/sys` entry.
* `dir` returns the directory contents of `/proc/` or `/sys/` directories.
* `netcache` returns a JSON object that contains NIC information. This file can be provided to other tools, if desired.
* `replay` start a replay on the `perfcollectord` hosts. Currently disabled.

Example to start collector (assumed with a configuration file):
```
$ perfprocessord start
```

Example of status:
```
$ perfprocessord status
Status             : 127.0.0.1:2222
Sink enabled       : false
Measurement enabled: true
Frequency          : 5s
Queue depth        : 1000
Queue free         : 1000
Systems            : [/proc/stat /proc/meminfo /proc/net/dev /proc/diskstats]
```

Example of stopping a collection:
```
$ perfprocessord stop
```

Example of status when there is no collection ongoing:
```
$ perfprocessord status
Status             : 127.0.0.1:2222
Sink enabled       : false
Measurement enabled: false
```

Example to obtain remote version by using the once command:
```
$ perfprocessord once systems=/proc/version
Linux version 5.8.18_1 (void-buildslave@a-hel-fi) (gcc (GCC) 9.3.0, GNU ld (GNU Binutils) 2.34) #1 SMP Sun Nov 1 14:25:13 UTC 2020
```

Example of obtaining a remote directory (note the required trailing slash):
```
$ perfprocessord dir directories=/proc/sys/net/unix/
Directory: /proc/sys/net/unix/
        max_dgram_qlen
```

Example of obtaining the netcache JSON object:
```
$ perfprocessord netcache run=0 > netcache.json
$ cat netcache.json
{"Site":1,"Host":0,"Run":0,"Measurement":{"Timestamp":"2020-12-07T09:09:47.38046831-06:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/net/eno1/duplex","Measurement":"full\n"}}
{"Site":1,"Host":0,"Run":0,"Measurement":{"Timestamp":"2020-12-07T09:09:47.38046831-06:00","Start":"0001-01-01T00:00:00Z","Duration":0,"Frequency":0,"System":"/sys/class/net/eno1/speed","Measurement":"1000\n"}}
```

##  perfjournal

The `perfjournal` tool is used to decrypt a collection journal.

Here is an example to decrypt a journal to JSON:
```
$ perfjournal --siteid=1 --sitename='Evil Corp' --license=6f37-6910-b2a0-e858-9657-f08d --input ~/.perfprocessord/data/journal --output x.json --mode=json -v
Total entries processed: 54340 in 4.642052411s
```

Here is an example to decrypt a journal to a directory structure that contains
CSV files:
```
$ mkdir ~/datacollection
$ perfjournal --siteid=1 --sitename='Evil Corp' --license=6f37-6910-b2a0-e858-9657-f08d --input ~/.perfprocessord/data/journal --output=~/datacollection --mode=csv -v
open /home/marco/xx/proc/stat
open /home/marco/xx/proc/meminfo
open /home/marco/xx/proc/net/dev
open /home/marco/xx/proc/diskstats
Entries processed: 23429
Entries processed: 46651
Total entries processed: 54340 in 11.633859494s
```

It is advisable to stop any collections and move the journal to a new location
before decrypting. Having a single journal per collection makes managing the
system a bit easier.

Note that the license information must exactly match the `perfprocessord.conf`
file or decryption will fail.

Note that `JSON` mode exports raw performance data wrapped in `JSON` objects
whereas CSV mode exports cubed (`sar` format) `CSV` files. The `JSON` object
format is as follows:
```
// WrapPCCollection wraps a raw collection in a site/host/run tuple.
type WrapPCCollection struct {
        Site        uint64
        Host        uint64
        Run         uint64
        Measurement *types.PCCollection
}

// PCCollection is a raw measurement that is sunk into the network.
type PCCollection struct {
	Timestamp   time.Time     // Time of *overall* collection
	Start       time.Time     // Start time of *this* collection
	Duration    time.Duration // Time collection took
	Frequency   time.Duration // Collection frequency
	System      string        // System that was measured
	Measurement string        // Raw measurement
}
```

## perfcpumeasure

The `perfcpumeasure` tool is used to collect CPU "unit" execution speeds for
every tenth percentile.
```
$ perfcpumeasure --siteid=1 --host=0 -v > training.json
=== looking for 10 busy 10.5 (load 10) units 72
=== looking for 20 busy 20.0 (load 20) units 136
=== looking for 30 busy 30.5 (load 30) units 208
=== looking for 40 busy 40.9 (load 40) units 280
=== looking for 50 busy 50.8 (load 50) units 344
=== looking for 60 busy 61.0 (load 60) units 416
=== looking for 70 busy 71.3 (load 70) units 488
=== looking for 80 busy 80.2 (load 80) units 552
=== looking for 90 busy 90.6 (load 90) units 624
$ cat training.json 
{"siteid":1,"host":0,"busy":10,"units":72}
{"siteid":1,"host":0,"busy":20,"units":136}
{"siteid":1,"host":0,"busy":30,"units":208}
{"siteid":1,"host":0,"busy":40,"units":280}
{"siteid":1,"host":0,"busy":50,"units":344}
{"siteid":1,"host":0,"busy":60,"units":416}
{"siteid":1,"host":0,"busy":70,"units":488}
{"siteid":1,"host":0,"busy":80,"units":552}
{"siteid":1,"host":0,"busy":90,"units":624}
{"siteid":1,"host":0,"busy":100,"units":688}
```

Note that `siteid` and `host` must match whatever is being replayed later. This
will need some automation but for now it must be provided manually.

## perfreplay

The `perfreplay` replay tool executes an attempted replica of the load found in
a journal. Again note that `host` and `run` must match earlier collected CPU
execution speeds.
```
$ perfreplay --siteid=1 --sitename='Evil Corp' --license=6f37-6910-b2a0-e858-9657-f08d --input=~/.perfprocessord/data/journal --host=0 --run=0 --output=- --log=prp=DEBUG --training=training.json
2020-12-07 15:35:10 INFO prp perfreplay.go:657 Start of day
2020-12-07 15:35:10 INFO prp perfreplay.go:658 Version 1.0.0 (Go version go1.15.5 linux/amd64)
2020-12-07 15:35:10 INFO prp perfreplay.go:660 Site   : Evil Corp
2020-12-07 15:35:10 INFO prp perfreplay.go:661 License: 6f37-6910-b2a0-e858-9657-f08d
2020-12-07 15:35:10 INFO prp perfreplay.go:662 Site ID: 1
2020-12-07 15:35:10 INFO prp perfreplay.go:663 Host ID: 0
2020-12-07 15:35:10 INFO prp perfreplay.go:664 Run ID : 0
2020-12-07 15:35:10 INFO prp perfreplay.go:541 workerStat: launched
2020-12-07 15:35:10 INFO prp perfreplay.go:484 workerMem: launched
```

At this time only CPU (`stat`) and memory (`meminfo`) are being replayed. The
replay ticks at the exact same frequency as the collection.
