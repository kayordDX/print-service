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
    private readonly FilePrinter printer;
    private static readonly EPSON e = new EPSON();
    private static Queue<List<byte[]>> printQueue = new();
    private static bool isPrinting = false;
    public Printer(ILogger<Printer> logger, IOptions<PrinterConfig> printerConfig, RedisClient redisClient)
    {
        _logger = logger;
        _redisClient = redisClient;
        _printerConfig = printerConfig.Value;
        printer = new FilePrinter(filePath: _printerConfig.FilePath);
        printer.StatusChanged += StatusChanged;
        printer.Write(e.Initialize());
        printer.Write(e.Enable());
        printer.Write(e.EnableAutomaticStatusBack());
    }

    public PrinterStatusEventArgs GetStatus()
    {
        return printer.Status;
    }

    public async Task RefreshStatusAsync()
    {
        _logger.LogInformation("Refreshing");
        var status = GetStatus();
        if (status == null)
        {
            _logger.LogError("Status was null - unable to read status from printer.");
            return;
        }

        PrinterStatus printerStatus = new()
        {
            DateUpdated = DateTime.Now,
            PrinterStatusEventArgs = status,
            PrinterConfig = _printerConfig
        };
        await SaveStatusToRedisAsync(printerStatus);
    }

    public void PrintQueue(List<byte[]> body)
    {
        printQueue.Enqueue(body);
        Print();
    }

    private void Print()
    {
        if (isPrinting) return;
        var status = GetStatus();
        if (status == null) return;

        try
        {
            if (!(status.IsPrinterOnline ?? false) || (status.IsInErrorState ?? false))
            {
                _logger.LogInformation("Printer not ready: IsOnline: {isOnline}, isErrorState: {errorState}", status.IsPrinterOnline, status.IsInErrorState);
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
            _logger.LogError("Status was null - unable to read status from printer.");
            return;
        }

        PrinterStatus printerStatus = new()
        {
            DateUpdated = DateTime.Now,
            PrinterStatusEventArgs = status,
            PrinterConfig = _printerConfig
        };
        SaveStatusToRedisAsync(printerStatus).ConfigureAwait(false);
        _logger.LogInformation("Printer Online Status: {status}", status.IsPrinterOnline);
        Print();
    }

    private async Task SaveStatusToRedisAsync(PrinterStatus printerStatus)
    {
        string key = $"printer:{_printerConfig.OutletId}:{_printerConfig.PrinterId}";
        await _redisClient.SetObjectAsync(key, printerStatus);
    }
}