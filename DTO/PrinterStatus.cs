using ESCPOS_NET;
using PrintService.Config;

namespace PrintService.DTO;

public class PrinterStatus
{
    public DateTime DateUpdated { get; set; } = DateTime.UtcNow;
    public PrinterConfig PrinterConfig { get; set; } = new();
    public PrinterStatusEventArgs? PrinterStatusEventArgs { get; set; } = null;
    public string? LastException { get; set; }
}