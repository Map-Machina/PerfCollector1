## perflicense

`perflicense` is a command line tool that is used to create licenses for
`perfprocessord` and stores them in a database.

Currently, it only supports UNIX sockets. A user must be created in the
database that has access to the database.

## Administrator

Create a UNIX user that will create licenses. In this example we are going with
`license`.

```
$ sudo su -
# useradd -m license
```

## Postgres setup

Assuming CentOS.
```
$ sudo su -
# yum install postgresql postgresql-server
# postgresql-setup initdb
# systemctl enable postgresql.service
# systemctl start postgresql.service
# sudo su - postgres
# psql
CREATE USER license WITH CREATEDB ENCRYPTED PASSWORD 'password';
```

Note: Do change the password.

## Create database

Prior to use the database must be created.
```
$ perflicense --createdb --dburi="user=license dbname=postgres host=/tmp/"
```

Note the `dbname=postgres` in `--dburi`.

## Create config file

Create `.perflicense/perflicense.conf` and add
```
dburi=user=license dbname=license host=/tmp/
```

Note the `dbname=license` in `--dburi`.

## Create  license administrator users

The license tool records which individual created a license and when. These
users are different from the database user (think user that logs in over a
website and does this work whereas the database user is what runs the service).

```
$ perflicense useradd email=marco@peereboom.us admin=true
User id: 1
```

Make note of the returned user id. In this example it is `1`. We will use this
number to identify who created a license later in this document.

## Create license

`perflicense` generates a license for `perfprocessord`. Pick one non-zero mac
address of the target machine.

Create or look up `site id` and `site name`.

For example:
```
$ perflicense licenseadd siteid=1337 sitename="Evil Corp" mac="00:22:4d:81:a1:41" duration=10 user=1
version not specified, defaulting to 1
# License information
siteid=1337
sitename=Evil Corp
license=9259-0f2b-8edf-48d2-ea05-ec6f
```

The output below the `# License information` must be copied into the config
file of the `perfprocessord` config file.

Example of getting the mac address from a machine:
```
$ cat /sys/class/net/eno1/address 
00:22:4d:81:a1:41
```

The output is what must be copied into the `perfprocessord.conf` file.
