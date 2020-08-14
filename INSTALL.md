# postgres

1. Install postgress either remote or locally
1. Make sure it launches as a service

# perfprocessord

1. Add rule to firewall for `egress 2222` (and `egress 5432` if you use remote postgres.)
1. Create normal user `perfprocessord`
1. `su` to `perfprocessord`
1. Create ssh key `ssh-keygen -t ed25519`. Don't setup a password on the ssh key. Make note of the line `SHA256:...`. Discard the username@machine name bit.
1. Copy `perfprocessord` to machine and make it run as a service as the newly created user.
1. Create directory `~/.perfprocessord`
1. Create configuration file `~/.perfprocessord/perfprocessord.conf`

```
# sshid is the file with ssh private key that is going to be used to connect to
# the collector.
sshid=~/.ssh/id_ed25519

# db is the database type. Only postgres is supported at this time.
db=postgres

# dburi is the connection URI to the database. This example is for postgres
# over UNIX sockets.
dburi=user=marco dbname=performancedata host=/tmp/

# hosts are all hosts this processor connects to. The format is
# site_id:machine_id/ip_address:port. Youy can sellect multiple lines but they
# must have a unique site_id:machine_id and ip_address.
hosts=0:0/127.0.0.1:2222 # site:machine/ip:port
#hosts=0:1/10.0.0.5:2222 # site:machine/ip:port
```

1. Create database. This example uses UNIX sockets
```
perfprocessord --dbcreate --db=postgres --dburi='user=postgres dbname=postgres host=/tmp/'
```

Note: you can print the ssh fingerprint with the following command:
```
ssh-keygen -l -f ~/.ssh/id_ed25519
256 SHA256:2WJ2Sv7cKpnDq+hyktlgMzVHIwBrgUDyEmQEGC7zTQQ marco@void (ED25519)
```
The only portion that matters is `SHA256:2WJ2Sv7cKpnDq+hyktlgMzVHIwBrgUDyEmQEGC7zTQQ`

# perfcollectord

1. Add rule to firewall for `ingress 2222`.
1. Create normal user `perfcollectord`
1. Copy `perfcollectord` to machine and make it run as a service as the newly created user.
1. Create directory `~/.perfcollectord`
1. Create configuration file `~/.perfcollectord/perfcollectord.conf`

Content of configuration file, replace allewedkeys with the `SHA256:...` you found during installation of `perfprocessord`:
```
sshid=/home/.ssh/id_ed25519
allowedkeys=SHA256:Rn2wwQetEJV/haY0qZXDu9p2zPPQw9pGi2Amiwuc9dE
```
