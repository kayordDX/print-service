using ESCPOS_NET;
using PrintService.Models;

namespace PrintService.Utils;

public static class Printer
{
    public static async Task Print(PrintMessage printMessage, ILogger logger)
    {
        try
        {
            var hostnameOrIp = printMessage.IPAddress;
            var port = printMessage.Port;
            var printer = new ImmediateNetworkPrinter(new ImmediateNetworkPrinterSettings()
            {
                ConnectionString = $"{hostnameOrIp}:{port}",
                PrinterName = printMessage.PrinterName,
            });
            await printer.WriteAsync(printMessage.PrintInstructions.ToArray());
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Error in Print");
        }
    }
}