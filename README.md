# Print Service

This is the print service to use to connect to existing EPSON printers using raspberry pi zero.

We used to get status from printer. Now we will fire and forget.

## Secets

```bash
dotnet user-secrets init
dotnet user-secrets set "ConnectionStrings:Redis" "secret" 
```

## Compose

```yaml
services:
  print-service:
    image: ghcr.io/kayorddx/print-service:latest
    environment:
      ConnectionStrings:Redis: pos.kayord.com,password=${REDIS_PASSWORD},ssl=False,abortConnect=False
      Serilog__MinimumLevel: Debug
    volumes:
      - /dev/usb/:/dev/usb/
      - ./config.yml:/app/config.yml
    restart: unless-stopped
    privileged: true

```

```yaml
services:
  print-service:
    image: ghcr.io/kayorddx/print-service:latest
    environment:
      ConnectionStrings:Redis: localhost,password=P@ss,ssl=False,abortConnect=False
      Printer:OutletId: 1
      Printer:PrinterId: 1
      Printer:Name: Main Printer
      Printer:RedisRefreshSec: 300
      Printer:StatusInitCheckSec: 300
      Printer:StatusCheckSec: 300
      Printer:FilePath: /dev/usb/lp0    
      Serilog__MinimumLevel: Debug
    devices:
      - /dev/usb/:/dev/usb/
    restart: unless-stopped
    privileged: true
```

## Find Devices in network

```bash
nmap -sn 192.168.1.*
```
