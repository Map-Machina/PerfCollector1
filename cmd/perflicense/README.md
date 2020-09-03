## perflicense

`perflicense` generates a license for `perfprocessord`. Pick one non-zero mac
address of the machine.

Create or look up `site id` and `site name`.

For example:
```
$ perflicense 1337 "Evil Corp" 00:22:4d:81:a1:41
siteid=1337
sitename=Evil Corp
license=27a0-13f3-1212-1379-e4d6-ca89
```

Example of getting the mac address from a machine:
```
$ cat /sys/class/net/eno1/address 
00:22:4d:81:a1:41
```

The output is what must be copied into the `perfprocessord.conf` file.
