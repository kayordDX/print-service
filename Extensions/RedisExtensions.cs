using PrintService.Utils;

namespace PrintService.Extensions;

public static class RedisExtensions
{
    public static IServiceCollection ConfigureRedis(this IServiceCollection services, IConfiguration configuration)
    {
        services.AddSingleton<RedisClient>();
        return services;
    }
}