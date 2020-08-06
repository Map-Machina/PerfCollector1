!/bin/bash

#This is s script that is intended to stress-ng and
#sar-sysstat stat collection.  

stress-ng  -c 2 --cpu-load 20 --vm 2 --vm-bytes 20% --hdd 2 --timeout=300s --metrics & sar -u -r -b -n DEV -o compareTotal 5 60

#stress-ng has been used to load multiple storage volumes.  It has to resident on the drive volume(s) that require testing

#convert the files sysstat files
sadf -d compareTotal > cpu_ext.csv  -- -u
sadf -d compareTotal > mem_ext.csv  -- -r
sadf -d compareTotal > file_ext.csv  -- -b
sadf -d compareTotal > net_ext.csv  -- -n DEV 

exit 0 
