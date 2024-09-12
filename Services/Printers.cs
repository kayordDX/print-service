using PrintService.Config;
using PrintService.Utils;

namespace PrintService.Services;

public class Printers
{
    private readonly ILogger<Printer> _logger;
    private readonly PrintersConfig _printersConfig = new PrintersConfig();
    private readonly RedisClient _redisClient;
    public List<Printer> printers = new();

    public Printers(ILogger<Printer> logger, RedisClient redisClient, Settings settings)
    {
        _logger = logger;
        _printersConfig = settings.GetPrintersConfig();
        _redisClient = redisClient;
    }

    public async Task InitPrinters()
    {
        foreach (PrinterConfig printerConfig in _printersConfig.Printers ?? [])
        {
            var printer = new Printer(_logger, printerConfig, _redisClient, _printersConfig);
            printers.Add(printer);
        }
        foreach (Printer printer in printers)
        {
            try
            {
                await printer.Initialize();
            }
            catch (Exception ex)
            {
                _logger.LogError("Could not initialize printer {name}: {ex}", printer.PrinterConfig.Name, ex.Message);
            }
        }
    }
}