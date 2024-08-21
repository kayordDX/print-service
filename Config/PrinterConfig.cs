namespace PrintService.Config;

public class PrinterConfig
{
    public int OutletId { get; set; }
    public int PrinterId { get; set; }
    public string Name { get; set; } = "Printer1";
    public string FilePath { get; set; } = "/dev/usb/lp0";
    public int RedisRefreshSec { get; set; } = 300;
    public int StatusInitCheckSec { get; set; } = 10;
    public int StatusCheckSec { get; set; } = 30;
}