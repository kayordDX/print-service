using CliWrap;
using CliWrap.Buffered;
using ESCPOS_NET;
using ESCPOS_NET.Emitters;
using PrintService.Config;
using PrintService.DTO;

namespace PrintService.Services;

public class Printer
{
    private readonly ILogger<Printer> _logger;
    private readonly PrinterConfig _printerConfig;
    private readonly PrintersConfig? _printersConfig;
    private readonly RedisClient _redisClient;
    private static BasePrinter? printer;
    private static readonly EPSON e = new EPSON();
    private static Queue<List<byte[]>> printQueue = new();
    private static bool isPrinting = false;
    private static PrinterStatusEventArgs? lastSyncStatus = null;
    private static Exception? lastException = null;
    public int PrinterId { get; set; }

    public Printer(ILogger<Printer> logger, PrinterConfig printerConfig, RedisClient redisClient, PrintersConfig? printersConfig)
    {
        var test = _printerConfig?.PrinterId;

        _logger = logger;
        _redisClient = redisClient;
        _printerConfig = printerConfig;
        _printersConfig = printersConfig;
        PrinterId = _printerConfig.PrinterId;
        InitPrinter();
    }

    private void InitPrinter()
    {
        try
        {
            if (_printerConfig?.IsUsbPrinter ?? false)
            {

                printer = new FilePrinter(filePath: _printerConfig.FilePath);
            }
            else
            {
                printer = new NetworkPrinter(new NetworkPrinterSettings { ConnectionString = $"{_printerConfig?.IPAddress}:{_printerConfig?.Port}", PrinterName = _printerConfig?.Name });
            }
            printer.StatusChanged += StatusChanged;
            printer.Connected += Connected;
            printer.Disconnected += Disconnected;
            printer.Write(e.Initialize());
            printer.Write(e.Enable());
            printer.Write(e.EnableAutomaticStatusBack());
        }
        catch (Exception ex)
        {
            lastException = ex;
            _logger.LogError("Could not initialize printer {ex}", ex.Message);
        }
    }

    public PrinterStatusEventArgs? GetStatus()
    {
        return printer?.Status;
    }

    private async Task<bool> GetPrinterFileStatus()
    {
        try
        {
            var result = await Cli.Wrap("/bin/cat")
            .WithArguments(["/dev/usb/lp0"])
            .WithValidation(CommandResultValidation.None)
            .ExecuteBufferedAsync();

            if (result.ExitCode == 0)
            {
                return true;
            }
            if (result.StandardError.Contains("Device or resource busy"))
            {
                return true;
            }
            return false;
        }
        catch (Exception ex)
        {
            lastException = ex;
            _logger.LogDebug("err {ex}", ex);
            return false;
        }
    }

    public bool ComparePrinterStatus(PrinterStatusEventArgs? first, PrinterStatusEventArgs? second)
    {
        if (first == null && second == null)
        {
            return first == second;
        }
        else if (first != null && second == null)
        {
            return false;
        }
        else if (first == null && second != null)
        {
            return false;
        }
        return first?.IsPrinterOnline == second?.IsPrinterOnline;
    }

    public async Task<bool> PrinterCheck()
    {
        _logger.LogDebug("Checking");
        _logger.LogDebug("Status {status}", GetStatus()?.IsPrinterOnline);
        _logger.LogDebug("LastSync {status}", lastSyncStatus?.IsPrinterOnline);
        bool checkResult = true;
        bool printFileStatus = true;
        if (_printerConfig?.IsUsbPrinter ?? false)
        {
            printFileStatus = await GetPrinterFileStatus();
        }

        bool noPrintPath = !printFileStatus;
        bool noPrinter = printer == null;
        bool shouldRefresh = false;

        if ((noPrintPath || noPrinter) && GetStatus() != null)
        {
            shouldRefresh = true;
        }
        else if (ComparePrinterStatus(GetStatus(), lastSyncStatus))
        {
            shouldRefresh = true;
        }

        if (noPrintPath)
        {
            checkResult = false;
            _logger.LogError("Print File Path not found");
            printer?.Dispose();
            printer = null;
        }
        if (noPrinter)
        {
            checkResult = false;
            InitPrinter();
        }

        if (shouldRefresh)
        {
            await RefreshStatusAsync();
        }
        return checkResult;
    }

    public async Task RefreshStatusAsync()
    {
        _logger.LogDebug("Refreshing");
        var status = GetStatus();
        PrinterStatus printerStatus = new()
        {
            DateUpdated = DateTime.UtcNow,
            PrinterStatusEventArgs = status,
            PrinterConfig = _printerConfig,
            LastException = (status == null) ? lastException?.Message : null
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
                        printer?.Write(row);
                    }
                }
            }
        }
        catch (Exception ex)
        {
            lastException = ex;
            _logger.LogError("Print error {error}", ex);
        }
        finally
        {
            isPrinting = false;
        }
    }

    private void StatusChanged(object? sender, EventArgs ps)
    {
        _logger.LogDebug("StatusChanged");
        var status = (PrinterStatusEventArgs?)ps;
        PrinterStatus printerStatus = new()
        {
            DateUpdated = DateTime.UtcNow,
            PrinterStatusEventArgs = status,
            PrinterConfig = _printerConfig,
            LastException = (status == null) ? lastException?.Message : null
        };
        SaveStatusToRedisAsync(printerStatus).ConfigureAwait(false);
        Print();
    }

    private void Connected(object? sender, EventArgs e)
    {
        _logger.LogDebug("Connected");
    }
    private void Disconnected(object? sender, EventArgs ps)
    {
        _logger.LogDebug("Disconnected");
    }

    private async Task SaveStatusToRedisAsync(PrinterStatus printerStatus)
    {
        lastSyncStatus = printerStatus.PrinterStatusEventArgs;
        string key = $"printer:{_printersConfig?.OutletId}:{_printerConfig.PrinterId}";
        await _redisClient.SetObjectAsync(key, printerStatus);
    }
}