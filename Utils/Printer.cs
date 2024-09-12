using CliWrap;
using CliWrap.Buffered;
using ESCPOS_NET;
using ESCPOS_NET.Emitters;
using PrintService.Config;
using PrintService.DTO;

namespace PrintService.Utils;

public class Printer
{
    private readonly ILogger<Printer> _logger;
    public readonly PrinterConfig PrinterConfig;
    private readonly PrintersConfig _printersConfig;
    private readonly RedisClient _redisClient;
    private BasePrinter? _printer;
    private readonly EPSON e = new EPSON();
    private Queue<List<byte[]>> printQueue = new();
    private bool isPrinting = false;
    private PrinterStatusEventArgs? lastSyncStatus = null;
    private DateTime lastSyncTime = DateTime.UtcNow;
    private Exception? lastException = null;
    private bool _initializedStatus = false;

    public Printer(ILogger<Printer> logger, PrinterConfig printerConfig, RedisClient redisClient, PrintersConfig printersConfig)
    {
        _logger = logger;
        _redisClient = redisClient;
        PrinterConfig = printerConfig;
        _printersConfig = printersConfig;
    }
    public async Task Initialize()
    {
        try
        {
            _logger.LogDebug("Initializing Printer: {name}", PrinterConfig.Name);
            if (PrinterConfig.IsUsbPrinter)
            {
                _printer = new FilePrinter(filePath: PrinterConfig.FilePath);
                var status = await GetPrinterFileStatus();
                if (status == true)
                {
                    if (!_initializedStatus)
                    {
                        InitializeStatus();
                    }
                }
            }
            else
            {
                _printer = new NetworkPrinter(new NetworkPrinterSettings
                {
                    ConnectionString = $"{PrinterConfig?.IPAddress}:{PrinterConfig?.Port}",
                    PrinterName = PrinterConfig?.Name,
                    ConnectedHandler = Connected,
                    DisconnectedHandler = Disconnected
                });
                _logger.LogDebug("New Network Printer: {ip}:{port}", PrinterConfig?.IPAddress, PrinterConfig?.Port);
            }
        }
        catch (Exception ex)
        {
            lastException = ex;
            _logger.LogError("Could not initialize printer {ex}", ex.Message);
            _printer = null;
        }
    }

    private void InitializeStatus()
    {
        if (_printer != null)
        {
            _printer.StatusChanged += StatusChanged;
            _printer.Write(e.Initialize());
            _printer.Write(e.Enable());
            _printer.Write(e.EnableAutomaticStatusBack());
            _initializedStatus = true;
        }
    }

    public PrinterStatusEventArgs? GetStatus()
    {
        _logger.LogDebug("GetStatus");
        if (_printer?.Status.IsPrinterOnline == null)
        {
            return null;
        }
        else
        {
            return _printer?.Status;
        }
    }

    private async Task<bool> GetPrinterFileStatus()
    {
        _logger.LogDebug("GetPrinterFileStatus");
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
            return true;
        }
        else if (first != null && second == null)
        {
            return false;
        }
        else if (first == null && second != null)
        {
            return false;
        }

        return (
            first?.IsPrinterOnline == second?.IsPrinterOnline &&
            first?.IsInErrorState == second?.IsInErrorState
        );
    }

    public async Task RefreshAsync()
    {
        _logger.LogDebug("Refreshing printer {name}", PrinterConfig.Name);
        _logger.LogDebug("Status {status}", GetStatus()?.IsPrinterOnline);
        _printer?.Write(e.EnableAutomaticStatusBack());
        await CheckProblems();
        await CheckSync();
    }

    private async Task CheckProblems()
    {
        bool shouldDispose = false;
        if (_printer == null)
        {
            _logger.LogError("Printer null: {name}", PrinterConfig.Name);
            shouldDispose = true;
        }

        // Long term status checks
        if (!shouldDispose)
        {

            // Long term status checks
            if (PrinterConfig.IsUsbPrinter)
            {
                bool isPrinterFile = await GetPrinterFileStatus();
                shouldDispose = !isPrinterFile;
            }
            else
            {
                // Network printer check
            }

        }

        // Last sync longer than stale time
        if (!shouldDispose)
        {
            var timeSpan = DateTime.UtcNow - lastSyncTime;
            if (timeSpan.TotalSeconds > _printersConfig.StaleSeconds)
            {
                if (GetStatus() == null)
                {
                    shouldDispose = true;
                }
            }
        }

        if (shouldDispose)
        {
            _printer?.Dispose();
            await Initialize();
            return;
        }
    }

    private async Task CheckSync()
    {
        await CheckSync(false);
    }

    private async Task CheckSync(bool forceSync)
    {
        var status = GetStatus();
        bool isUnchanged = ComparePrinterStatus(status, lastSyncStatus);
        bool shouldSync = false;
        if (forceSync)
        {
            shouldSync = true;
        }
        if (!isUnchanged)
        {
            // Status Changed so sync with redis immediately
            shouldSync = true;
        }
        else
        {
            // Check if we have not already synced same status in last minute            
            var timeSpan = DateTime.UtcNow - lastSyncTime;
            if (timeSpan.TotalSeconds > _printersConfig.RedisSyncSeconds)
            {
                shouldSync = true;
            }
        }

        if (shouldSync)
        {

            PrinterStatus printerStatus = new()
            {
                DateUpdated = DateTime.UtcNow,
                PrinterStatusEventArgs = status,
                Name = PrinterConfig.Name,
                PrinterId = PrinterConfig.PrinterId,
                OutletId = _printersConfig.OutletId,
                LastException = (status == null) ? lastException?.Message : null
            };
            await SaveStatusToRedisAsync(printerStatus);
        }
    }

    public void PrintQueue(List<byte[]> body)
    {
        _logger.LogDebug("Adding to print queue");
        printQueue.Enqueue(body);
        Print();
    }

    private void Print()
    {
        _logger.LogDebug("Printing...");
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
                        _printer?.Write(row);
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
        CheckSync().ConfigureAwait(false);

        var status = (PrinterStatusEventArgs?)ps;
        _logger.LogDebug("Status Online {status}", status?.IsPrinterOnline);
        _logger.LogDebug("Has Paper? {status}", status?.IsPaperOut);
        _logger.LogDebug("Paper Running Low? {status}", status?.IsPaperLow);
        _logger.LogDebug("Cash Drawer Open? {status}", status?.IsCashDrawerOpen);
        _logger.LogDebug("Cover Open? {status}", status?.IsCoverOpen);

        Print();
    }

    private void Connected(object? sender, EventArgs ps)
    {
        _logger.LogDebug("Connected");
        if (!_initializedStatus)
        {
            InitializeStatus();
        }
        else
        {
            _printer?.Write(e.EnableAutomaticStatusBack());
        }
        Print();
    }
    private void Disconnected(object? sender, EventArgs ps)
    {
        _logger.LogDebug("Disconnected");
        CheckSync(true).ConfigureAwait(false);
    }

    private async Task SaveStatusToRedisAsync(PrinterStatus printerStatus)
    {
        _logger.LogDebug("SaveStatusToRedisAsync");
        lastSyncStatus = printerStatus.PrinterStatusEventArgs;
        lastSyncTime = DateTime.UtcNow;
        string key = $"printer:{_printersConfig?.OutletId}:{PrinterConfig.PrinterId}";
        await _redisClient.SetObjectAsync(key, printerStatus);
    }
}