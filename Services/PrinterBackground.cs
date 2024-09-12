using PrintService.Config;

namespace PrintService.Services;

public class PrinterBackground : BackgroundService
{
    private readonly ILogger<PrinterBackground> _logger;
    private readonly Printers _printers;
    private readonly PrintersConfig _printersConfig = new PrintersConfig();

    public PrinterBackground(ILogger<PrinterBackground> logger, Printers printers, Settings settings)
    {
        _logger = logger;
        _printers = printers;
        _printersConfig = settings.GetPrintersConfig();
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        _logger.LogDebug("Starting PrinterBackground");
        await _printers.InitPrinters();
        while (!stoppingToken.IsCancellationRequested)
        {
            foreach (var printer in _printers.printers)
            {
                await printer.RefreshAsync();
            }
            await Task.Delay(TimeSpan.FromSeconds(_printersConfig.RefreshSeconds), stoppingToken);
        }
    }
}
