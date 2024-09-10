namespace PrintService.Config;

public class PrinterConfig
{
    public int PrinterId { get; set; }
    public string Name { get; set; } = "Printer1";
    public string FilePath { get; set; } = "/dev/usb/lp0";
    public string IPAddress { get; set; } = "10.0.0.3";
    public int Port { get; set; } = 9100;
    public bool IsUsbPrinter { get; set; } = true;
    public int OutletId { get; set; }
}