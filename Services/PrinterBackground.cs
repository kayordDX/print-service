namespace PrintService.Services;

public class PrinterBackground : BackgroundService
{
    private readonly ILogger<PrinterBackground> _logger;
    private readonly Printer _printer;
    public PrinterBackground(ILogger<PrinterBackground> logger, Printer printer)
    {
        _logger = logger;
        _printer = printer;
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            await Task.Delay(TimeSpan.FromMinutes(5), stoppingToken);
            await _printer.RefreshStatusAsync();
        }
    }
}