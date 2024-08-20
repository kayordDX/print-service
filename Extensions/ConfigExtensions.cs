using PrintService.Config;

namespace PrintService.Extensions;

public static class ConfigExtensions
{
    public static IServiceCollection ConfigureConfig(this IServiceCollection services, IConfiguration configuration)
    {
        services.Configure<PrinterConfig>(configuration.GetSection("Printer"));
        return services;
    }
}