namespace PrintService.Config;

public class PrintersConfig
{
    public int OutletId { get; set; }
    public List<PrinterConfig> Printers { get; set; } = new List<PrinterConfig>();
    public int RefreshSeconds { get; set; } = 20;
    public int RedisSyncSeconds { get; set; } = 60;
    public int StaleSeconds { get; set; } = 300;
}