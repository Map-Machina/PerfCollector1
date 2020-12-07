# Performance Monitoring and replay tools

These tools are at this time strictly Linux only. Substantial effort must be
made to convert them over to other OS'.

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

## Instalation

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

# perfcollector
Performance data collector and processor.

## perfcollectord

```
perfcollectord --sshid=/home/marco/.ssh/id_ed25519 --listen=127.0.0.1:2222 --allowedkeys=SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE
```

All command line switches can also be stored in a configuration file. The
format is always ~/.TOOLNAME/$TOOLNAME.conf. For example,
`.perfprocessord/perfprocessord.conf` Any lines prefixed with `#` are ignored.

Here is an example:
```
$ cat /home/marco/.perfprocessord/perfprocessord.conf 
#[Application Options]

#sshid=~/.ssh/id_ed25519

journal=1

hosts=1:0/127.0.0.1:2222
#hosts=1:1/10.170.0.5:2222

siteid=1
sitename=Evil Corp
license=6f37-6910-b2a0-e858-9657-f08d
```

Note that this example cannot be used verbatim because the license has a
time bomb builtin. The license must be generated every time for each
deployment.

## perfprocessord in sink mode (required database)
```
perfprocessord --sshid=/home/marco/.ssh/id_ed25519'
```

## perfprocessord single shot commands
* `start` start performance data collection
* `stop` stop performance data collection
* `status` returns collector status

Example to start collector:
```
perfprocessord --sshid=/home/marco/.ssh/id_ed25519 --host=127.0.0.1:2222 --debuglevel=trace start
```

## Print ssh fingerprint example
```
$ ssh-keygen -l -f /home/marco/.ssh/id_ed25519 -t SHA256
256 SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE marco@void (ED25519)
```

##  perfjournal
```
$ perfjournal --siteid=1 --sitename='Evil Corp' --license=6f37-6910-b2a0-e858-9657-f08d --input /home/marco/.perfprocessord/data/journal --output x.json --mode=json  -v
```

## perfcpumeasure
```
$ perfcpumeasure --siteid=1 --host=0 -v > training.json
```

## perfreplay
```
perfreplay --siteid=1 --sitename='Evil Corp' --license=6f37-6910-b2a0-e858-9657-f08d --input /home/marco/.perfprocessord/data/journal --host=0 --run=0 --output=- --log=prp=DEBUG --training=training.json
```
