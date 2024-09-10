using Microsoft.Extensions.Options;
using PrintService.Config;

namespace PrintService.Services;

public class PrinterBackground : BackgroundService
{
    private readonly ILogger<PrinterBackground> _logger;
    private readonly Printers _printers;
    private readonly PrintersConfig? _printersConfig;

    public PrinterBackground(ILogger<PrinterBackground> logger, Printers printers, Settings settings)
    {
        _logger = logger;
        _printers = printers;
        _printersConfig = settings.GetPrintersConfig();
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            foreach (var printer in _printers.printers)
            {
                await printer.RefreshStatusAsync();
            }
            await Task.Delay(TimeSpan.FromSeconds(_printersConfig?.RedisRefreshSec ?? 300), stoppingToken);
        }
    }
}
