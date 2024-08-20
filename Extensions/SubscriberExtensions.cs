using PrintService.Services;

namespace PrintService.Extensions;

public static class SubscriberExtensions
{
    public static IServiceCollection ConfigureSubscriber(this IServiceCollection services, IConfiguration configuration)
    {
        services.AddSingleton<Subscriber>();
        return services;
    }
}