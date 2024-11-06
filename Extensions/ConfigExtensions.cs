using PrintService.Models;

namespace PrintService.Extensions;

public static class ConfigExtensions
{
    public static IServiceCollection ConfigureConfig(this IServiceCollection services, IConfiguration configuration)
    {
        services.Configure<Config>(configuration.GetSection("Config"));
        return services;
    }
}