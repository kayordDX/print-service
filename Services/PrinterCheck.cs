using PrintService.Config;

namespace PrintService.Services;

public class PrinterCheck : BackgroundService
{
    private readonly ILogger<PrinterCheck> _logger;
    private readonly Printers _printers;
    private readonly PrintersConfig _printersConfig;
    public PrinterCheck(ILogger<PrinterCheck> logger, Printers printers, Settings settings)
    {
        _logger = logger;
        _printers = printers;
        _printersConfig = settings.GetPrintersConfig() ?? new PrintersConfig();
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            await Task.Delay(TimeSpan.FromSeconds(_printersConfig.StatusCheckSec), stoppingToken);
            foreach (var printer in _printers.printers)
            {
                await printer.PrinterCheck();
            }
        }
    }
}