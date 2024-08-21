using Microsoft.Extensions.Options;
using PrintService.Config;

namespace PrintService.Services;

public class PrinterCheck : BackgroundService
{
    private readonly ILogger<PrinterCheck> _logger;
    private readonly Printer _printer;
    private readonly PrinterConfig _printerConfig;
    public PrinterCheck(ILogger<PrinterCheck> logger, Printer printer, IOptions<PrinterConfig> printerConfig)
    {
        _logger = logger;
        _printer = printer;
        _printerConfig = printerConfig.Value;
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            if (_printer.GetStatus() == null)
            {
                await Task.Delay(TimeSpan.FromSeconds(_printerConfig.StatusInitCheckSec), stoppingToken);
            }
            else
            {
                await Task.Delay(TimeSpan.FromSeconds(_printerConfig.StatusCheckSec), stoppingToken);
            }
            await _printer.PrinterCheck();
        }
    }
}