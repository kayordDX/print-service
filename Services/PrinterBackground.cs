using Microsoft.Extensions.Options;
using PrintService.Config;

namespace PrintService.Services;

public class PrinterBackground : BackgroundService
{
    private readonly ILogger<PrinterBackground> _logger;
    private readonly Printer _printer;
    private readonly PrinterConfig _printerConfig;
    public PrinterBackground(ILogger<PrinterBackground> logger, Printer printer, IOptions<PrinterConfig> printerConfig)
    {
        _logger = logger;
        _printer = printer;
        _printerConfig = printerConfig.Value;
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            await Task.Delay(TimeSpan.FromSeconds(_printerConfig.RedisRefreshSec), stoppingToken);
            await _printer.RefreshStatusAsync();
        }
    }
}