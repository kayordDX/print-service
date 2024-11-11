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
      ConnectionStrings:Redis: localhost,password=P@ss,ssl=False,abortConnect=False
      Config:OutletIds: 1,2
      Config:DeviceId: 1
      Serilog__MinimumLevel: Debug
    restart: unless-stopped
```

## Find Devices in network

```bash
nmap -sn 192.168.1.*
```
