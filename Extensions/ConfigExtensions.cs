using PrintService.Config;

namespace PrintService.Extensions;

public static class ConfigExtensions
{
    public static IServiceCollection ConfigureConfig(this IServiceCollection services, IConfiguration configuration)
    {
        services.Configure<MainConfig>(configuration.GetSection("Config"));
        return services;
    }
}