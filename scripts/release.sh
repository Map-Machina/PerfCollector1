#!/bin/sh

# Simple bash script to build perfcollector binaries.

COMMIT=`git rev-parse --short HEAD`
FLAGS="-X main.appBuild=$COMMIT"

# If no tag specified, use date + version otherwise use tag.
if [[ $1x = x ]]; then
    DATE=`date +%Y%m%d`
    VERSION="01"
    TAG=$DATE-$VERSION
else
    TAG=$1
fi

PACKAGE=perfcollector
MAINDIR=$PACKAGE-$TAG
mkdir -p $MAINDIR
cd $MAINDIR

SYS="linux-386 linux-amd64"

# Use the first element of $GOPATH in the case where GOPATH is a list
# (something that is totally allowed).
GPATH=$(echo $GOPATH | cut -f1 -d:)

for i in $SYS; do
    OS=$(echo $i | cut -f1 -d-)
    ARCH=$(echo $i | cut -f2 -d-)
    mkdir $PACKAGE-$i-$TAG
    cd $PACKAGE-$i-$TAG
    echo "Building:" $OS $ARCH
# Add CGO_ENABLED=0 so that the files are statically linked
    env CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -ldflags "${FLAGS}" github.com/businessperformancetuning/perfcollector/cmd/perfcollectord
    env CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -ldflags "${FLAGS}" github.com/businessperformancetuning/perfcollector/cmd/perfprocessord
#    env CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -ldflags "${FLAGS}" github.com/businessperformancetuning/perfcollector/cmd/perflicense
#    env CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -ldflags "${FLAGS}" github.com/businessperformancetuning/perfcollector/cmd/perfjournal
    #cp $GPATH/src/github.com/businessperformancetuning/perfcollector/cmd/perfcollectord/perfcollectord.conf .
    cd ..
    if [[ $OS = "windows" ]]; then
	zip -r $PACKAGE-$i-$TAG.zip $PACKAGE-$i-$TAG
    else
	tar -cvzf $PACKAGE-$i-$TAG.tar.gz $PACKAGE-$i-$TAG
    fi
    rm -r $PACKAGE-$i-$TAG
done

sha256sum * > manifest-$TAG.txt
