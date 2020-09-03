## perfjournal

`perfjournal` decrypts an encrypted journal file when provided with the appropriate site and VM specific values.

Example:
```
$ perfjournal 1337 "Evil Corp" 27a0-13f3-1212-1379-e4d6-ca89 /home/marco/.
perfprocessord/data/journal
```

The provided parameters MUST exactly match the site_id, site_name and license
for the VM. The encryption key is derived from that data and without it, it
cannot decrypt the journal.

See `cmd/perflicense` for a license example.
