# CS463 network check: runs ON the reader, read-only, ~8 s healthy / ~1 min blocked.
# Shows where the path to the broker stops: clock, DNS, TCP, TLS, NTP, logs.
# How to run it and read the output: cs463-network-check.md (same directory).
# B overrides the broker host, e.g. B=mqtt.preview.gke.trakrf.id bash cs463-network-check.sh
B=${B:-mqtt.prod.gke.trakrf.id}
echo "=== 1. clock (should be today, UTC)"; date -u; timedatectl 2>/dev/null | grep -i synchronized
echo "=== 2. reader address, gateway, DNS servers from DHCP"
ip -4 addr show eth0 | grep inet; ip route | grep default
W=$(connmanctl services | awk '/Wired/{print $NF}'); connmanctl services "$W" | grep -E '^ *Nameservers '
echo "=== 3. DNS: reader's local resolver, then a public resolver"
timeout 10 nslookup "$B" 127.0.0.1 2>&1 | tail -n 3
timeout 10 nslookup "$B" 8.8.8.8 2>&1 | tail -n 3
echo "=== 4. TCP reachability (open / blocked)"
for t in "$B 8883" "www.google.com 443" "app.trakrf.id 443"; do
  set -- $t
  if timeout 10 bash -c "</dev/tcp/$1/$2" 2>/dev/null; then echo "$1:$2 open"; else echo "$1:$2 BLOCKED or unreachable"; fi
done
echo "=== 5. who signs the broker's certificate (expect Let's Encrypt)"
O=$(timeout 15 openssl s_client -connect "$B:8883" -servername "$B" </dev/null 2>&1 | grep -E '^ *(s|i):|Verify return code|connect:errno' | head -n 6); echo "${O:-no TLS answer (blocked, timed out or DNS failed)}"
echo "=== 6. public IP the site uses on the internet"
timeout 15 curl -s https://ifconfig.me; echo
echo "=== 7. NTP (UDP 123)"
timeout 15 ntpdate -q pool.ntp.org 2>&1 | tail -n 1
echo "=== 8. recent vendor MQTT errors / connects (no tag data)"
grep -E 'MQTTClient|Exception' /opt/EmbeddedGlassFish/logs/CS463System.log | grep -vE 'PUBLISH |queuePublish' | tail -n 8 | cut -c1-220
echo "=== 9. TrakRF daemon log"
journalctl -u mqtt-rpcd -n 8 --no-pager 2>&1 | cut -c1-220
echo "=== done"
