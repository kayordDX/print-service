namespace PrintService.Config;

public class PrintersConfig
{
    public int OutletId { get; set; }
    public List<PrinterConfig> Printers { get; set; } = new List<PrinterConfig>();
    public int RedisRefreshSec { get; set; } = 300;
    public int StatusCheckSec { get; set; } = 30;
}