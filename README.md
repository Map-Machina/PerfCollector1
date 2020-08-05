# perfcollector
Performance data collector and processor.

## perfcollectord
```
perfcollectord --sshid=/home/marco/.ssh/id_ed25519 --listen=127.0.0.1:2222 --allowedkeys=SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE
```

## perfprocessord database creation

`perfprocessord` requires a database to operate in sink mode. The first time it
is run it must be called with the database creation paramaters. This example
uses postgres over a unix socket.
```
perfprocessord --dbcreate --db=postgres --dburi='user=marco dbname=postgres host=/tmp/'
```

## perfprocessord in sink mode (required database)
```
perfprocessord --sshid=/home/marco/.ssh/id_ed25519 --db=postgres --dburi='user=marco dbname=performancedata host=/tmp/'
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

## TODO

## Intalling postgres

This is just an example on void linux.

```
sudo xbps-install -S postgresql postgresql-contrib
sudo ln -s /etc/sv/postgresql /var/service
sudo passwd postgres
sudo mkdir /home/postgres
sudo chown -R postgres:postgres /home/postgres
sudo su - postgres
initdb -D /home/postgres/
PGDATA = /home/postgres /usr/bin/pg_ctl -D /home/postgres -l logfile start
```
