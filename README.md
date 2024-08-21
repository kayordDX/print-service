# Print Service

This is the print service to use to connect to existing EPSON printers using raspberry pi zero

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
      ASPNETCORE_Printer:OutletId: 1
      ASPNETCORE_Printer:PrinterId: 1
      ASPNETCORE_Printer:Name: Main Printer
      # ASPNETCORE_Printer:FilePath: /dev/usb/lp0      
    devices:
      - /dev/usb/lp0:/dev/usb/lp0
    restart: unless-stopped
    privileged: true
```