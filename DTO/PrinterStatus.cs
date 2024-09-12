using ESCPOS_NET;

namespace PrintService.DTO;

public class PrinterStatus
{
    public DateTime DateUpdated { get; set; } = DateTime.UtcNow;
    public PrinterStatusEventArgs? PrinterStatusEventArgs { get; set; } = null;
    public string? LastException { get; set; }
    public int PrinterId { get; set; }
    public string Name { get; set; } = string.Empty;
    public int OutletId { get; set; }
}