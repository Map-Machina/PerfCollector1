# perfcollector
Performance data collector and processor.

## perfcollectord
```
perfcollectord --user=marco --sshid=/home/marco/.ssh/id_ed25519 --listen=127.0.0.1:2222 --allowedkeys=SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE --debuglevel=trace
```

## perfprocessord in sink mode
```
perfprocessord --user=marco --sshid=/home/marco/.ssh/id_ed25519 --host=127.0.0.1:2222 --debuglevel=trace sink
```

## perfprocessord single shot commands
* `start` start performance data collection
* `stop` stop performance data collection
* `status` returns collector status

Example to start collector:
```
perfprocessord --user=marco --sshid=/home/marco/.ssh/id_ed25519 --host=127.0.0.1:2222 --debuglevel=trace start
```

## Print ssh fingerprint example
```
$ ssh-keygen -l -f /home/marco/.ssh/id_ed25519 -t SHA256
256 SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE marco@void (ED25519)
```

## TODO
* Remove `--sshid` and only use ssh keys
