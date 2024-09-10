using Microsoft.Extensions.Options;
using PrintService.Config;

namespace PrintService.Services;

public class Printers
{
    private readonly ILogger<Printer> _logger;
    private readonly PrintersConfig? _printersConfig;
    private readonly RedisClient _redisClient;
    public List<Printer> printers = new();

    public Printers(ILogger<Printer> logger, RedisClient redisClient, Settings settings)
    {
        _logger = logger;
        _printersConfig = settings?.GetPrintersConfig();
        _redisClient = redisClient;
        InitPrinters();
    }

    private void InitPrinters()
    {
        foreach (PrinterConfig printer in _printersConfig?.Printers ?? [])
        {
            if (printer != null)
            {
                printers.Add(new Printer(_logger, printer, _redisClient, _printersConfig));
            }
        }
    }
}