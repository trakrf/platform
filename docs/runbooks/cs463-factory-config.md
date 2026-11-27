# Clearing a CS463's factory config

Step 5 of [Commissioning a CS463](commissioning-a-cs463.md): remove everything
on a factory-fresh reader that publishes somewhere. `$B`, `$S` and the backup
come from §0 and §4 of that page.

The factory image carries demo CloudServers (HTTP, TCP, and an enabled MQTT
entry with a random client id) and the actions, heartbeats and notifications
that point at them. Left in place they collide with the golden config; a
leftover MQTT entry caused client-id collisions on an earlier reader.

**Only delete things that publish.** GPIO, display and USB examples publish
nothing; leave them alone, as the daemon does.

The reader refuses to delete anything still referenced, so map the references
first (`listServer`, `listEvent`, `listResultantAction`, `listHeartBeat`,
`listPowerUpNotification`, `listReaderErrorNotification`), then delete from the
leaves inward. The ids contain spaces and parentheses, so URL-encode them:

```sh
del() { curl -s -G "$B" --data-urlencode "session_id=$S" \
          --data-urlencode "command=$1" --data-urlencode "$2=$3"; echo; }
```

| Order | What | Command | Id parameter |
|---|---|---|---|
| 1 | Events whose `resultant_action` sends to a server | `delEvent` | `event_id` |
| 2 | Actions with a non-empty `server_id` | `delResultantAction` | `action_id` |
| 3 | Heartbeats with a non-empty `server_id` | `delHeartBeat` | `heart_beat_id` |
| 4 | Power-up notifications | `delPowerUpNotification` | `notification_id` |
| 5 | Reader-error notifications | `delReaderErrorNotification` | `notification_id` |
| 6 | Every CloudServer | `delServerID` | `server_id` |

On the factory image that came to four events, five actions, one heartbeat, one
power-up and one reader-error notification, and four servers. Heartbeats with
no server (the ARP and ping ones) stay.

Then check the three "Done means" conditions in §5 of the commissioning page.
