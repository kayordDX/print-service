using System.Text.Json;
using ESCPOS_NET;
using ESCPOS_NET.Emitters;
using Microsoft.Extensions.Options;
using PrintService.Config;
using PrintService.DTO;

namespace PrintService.Services;

public class Printer
{
    private readonly ILogger<Printer> _logger;
    private readonly PrinterConfig _printerConfig;
    private readonly RedisClient _redisClient;
    private static readonly FilePrinter printer = new FilePrinter(filePath: "/dev/usb/lp0");
    private static readonly EPSON e = new EPSON();
    private static PrinterStatusEventArgs? lastStatus = null;
    private static Queue<List<byte[]>> printQueue = new();
    private static bool isPrinting = false;
    public Printer(ILogger<Printer> logger, IOptions<PrinterConfig> printerConfig, RedisClient redisClient)
    {
        _logger = logger;
        _redisClient = redisClient;
        _printerConfig = printerConfig.Value;
        printer.StatusChanged += StatusChanged;
        printer.Write(e.Initialize());
        printer.Write(e.Enable());
        printer.Write(e.EnableAutomaticStatusBack());
    }

    public PrinterStatusEventArgs? GetStatus()
    {
        return lastStatus;
    }

    public void PrintQueue(List<byte[]> body)
    {
        printQueue.Enqueue(body);
        Print();
    }

    private void Print()
    {
        if (isPrinting) return;
        if (lastStatus == null) return;

        try
        {
            if (!(lastStatus.IsPrinterOnline ?? false) || (lastStatus.IsInErrorState ?? false))
            {
                _logger.LogInformation("Printer not ready: IsOnline: {isOnline}, isErrorState: {errorState}", lastStatus.IsPrinterOnline, lastStatus.IsInErrorState);
            }
            else
            {
                while (printQueue.Count() > 0)
                {
                    isPrinting = true;
                    var body = printQueue.Dequeue();
                    foreach (var row in body)
                    {
                        printer.Write(row);
                    }
                }
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex.Message);
        }
        finally
        {
            isPrinting = false;
        }
    }

    private void StatusChanged(object? sender, EventArgs ps)
    {
        var status = (PrinterStatusEventArgs)ps;
        if (status == null)
        {
            lastStatus = null;
            _logger.LogError("Status was null - unable to read status from printer.");
            return;
        }
        lastStatus = status;

        PrinterStatus printerStatus = new()
        {
            DateUpdated = DateTime.Now,
            PrinterStatusEventArgs = status,
            PrinterConfig = _printerConfig
        };
        string key = $"printer:{_printerConfig.OutletId}:{_printerConfig.PrinterId}";
        SaveStatusToRedis(key, printerStatus).ConfigureAwait(false);
        _logger.LogInformation("Printer Online Status: {status}", status.IsPrinterOnline);
        Print();
    }

    private async Task SaveStatusToRedis(string key, PrinterStatus printerStatus)
    {
        await _redisClient.SetObjectAsync(key, printerStatus);
    }
}