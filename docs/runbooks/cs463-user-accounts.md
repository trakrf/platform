# CS463 user accounts

Part of [commissioning a CS463](commissioning-a-cs463.md), §2 and §8. A
commissioned reader has three web/API accounts and no others:

| Account | Who | Required |
|---|---|---|
| `root` | the commissioning operator | yes |
| `trakrf` | `mqtt-rpcd` | yes |
| an operator account | the site's IT or facilities contact | only if they ask for one |

**The reader allows one login session per user, not one in total.** The daemon
gets its own account so that a person logged in as `root`, through the web UI or
`/API`, never takes the daemon's session slot, and the daemon never takes theirs.

Every command below runs with a `root` session `$S` and the `$B` from §0 of the
commissioning page. Each permission is a bitmask: 1 view, 2 change, 4 run.

## Remove the factory example user

The factory image ships an observer account, `ExampleUser1PWDcsl`, whose
password is `csl`. Changing root's password (§2) leaves it working. Delete it:

```sh
curl -s -G "$B" --data-urlencode "session_id=$S" \
    --data-urlencode "command=delUser" --data-urlencode "username=ExampleUser1PWDcsl"
```

`listUsers` should no longer show it.

## Add a user

`addUser` takes `username`, `password` and `desc`, plus one parameter per
permission. Write the password to a mode-600 file and send it with
`--data-urlencode "password@<file>"`, so it never reaches argv:

```sh
P="AccessMode=3 CapturePointName=3 …"        # the line for the account, below
set --; for kv in $P; do set -- "$@" --data-urlencode "$kv"; done
curl -s -G "$B" --data-urlencode "session_id=$S" --data-urlencode "command=addUser" \
    --data-urlencode "username=trakrf" --data-urlencode "password@$PASS_FILE" \
    --data-urlencode "desc=TrakRF mqtt-rpcd daemon" "$@"
```

Check what landed with `listUsers`, not the `<Ack>`.

### `trakrf`: the daemon

Everything `root` has except user management, firmware upgrade and
configuration backup/restore. The daemon needs none of those, and none of them
should be reachable with a password that sits in a file on the reader.

```
AccessMode=3 CapturePointName=3 CloudServer=3 CompanyLabel=3 ConfigurationBackupRestore=0
ConfigureLNAGain=3 CustomEmbeddedApplication=3 DataFormat=3 DisplayFormat=3 DownloadLogFile=4
EthernetWiFiConfiguration=3 Event=3 FirmwareUpgrade=0 ForceLogout=4 FrequencyConfiguration=3
HeartBeatNotification=3 IOPortControl=7 LogFileConfiguration=3 MemoryInformation=3
OperationProfile=3 PowerUpNotification=3 ReadTag=4 ReaderErrorNotification=3 ReaderId=3
RebootSystem=4 ResultantAction=3 SSLCertificate=3 ScheduledReboot=3 Status=1 TagDatabase=3
TagFilter=3 TagGroup=3 TagInventory=1 TimeSetting=3 Trigger=3 UserManagement=0 WriteTag=4
```

No `/API` command is reserved for `root`: each one checks the caller's password
and permissions. The error `not a root user` is the reader's generic
authentication failure, so it means a wrong password, not a missing privilege.
`Reader.GetCapabilities`, `Reader.GetOperProfile`, `Reader.SetOperProfile`
(which writes through the web servlet) and `Gpo.Set` were verified as `trakrf`.

The daemon reads the account from its env file (§8): `READER_API_USER=trakrf`
and `READER_API_PASS`. Without `READER_API_USER` it logs in as `root`.

### Operator account

For a site contact who wants to manage the reader's network. This one can change
network, time and RFID settings, reboot, and download logs. It can view
everything else, but cannot touch the CloudServer, events, actions, triggers,
data format, firmware, users, backup/restore or force-logout, so it cannot break
the path to the platform:

```
AccessMode=3 CapturePointName=3 CloudServer=0 CompanyLabel=1 ConfigurationBackupRestore=0
ConfigureLNAGain=3 CustomEmbeddedApplication=1 DataFormat=0 DisplayFormat=1 DownloadLogFile=4
EthernetWiFiConfiguration=3 Event=0 FirmwareUpgrade=0 ForceLogout=0 FrequencyConfiguration=3
HeartBeatNotification=1 IOPortControl=1 LogFileConfiguration=1 MemoryInformation=1
OperationProfile=3 PowerUpNotification=1 ReadTag=4 ReaderErrorNotification=1 ReaderId=1
RebootSystem=4 ResultantAction=0 SSLCertificate=1 ScheduledReboot=1 Status=1 TagDatabase=1
TagFilter=3 TagGroup=1 TagInventory=1 TimeSetting=3 Trigger=0 UserManagement=0 WriteTag=4
```

The web UI shows `Access right not granted` on the pages it cannot change.
A site contact may want a memorable password for this account, and that is a
reasonable trade on a reader whose root and daemon passwords are strong. Keep it
in the password manager with the others.

**`forceLogout` logs out every user, not just one.** It needs the ForceLogout
permission, so the operator account cannot use it to knock the daemon off.
