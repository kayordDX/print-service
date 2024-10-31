using System.Text.Json;
using Microsoft.Extensions.Options;
using PrintService.Config;
using PrintService.Utils;
using StackExchange.Redis;

namespace PrintService.Services;

public class Subscriber : BackgroundService
{
    private readonly ILogger<Worker> _logger;
    private readonly Printers _printers;
    private readonly RedisClient _redisClient;
    private readonly PrintersConfig? _settings;

    public Subscriber(ILogger<Worker> logger, RedisClient redisClient, Printers printers, Settings settings)
    {
        _logger = logger;
        _redisClient = redisClient;
        _printers = printers;
        _settings = settings.GetPrintersConfig();
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        foreach (var printer in _printers.printers)
        {
            RedisChannel channel = new RedisChannel($"print:{printer.PrinterConfig.OutletId}:{printer.PrinterConfig.PrinterId}", RedisChannel.PatternMode.Auto);
            var subscriber = await _redisClient.GetSubscriber();

            await subscriber.SubscribeAsync(channel, (channel, message) =>
            {
                if (message.IsNullOrEmpty)
                {
                    return;
                }

                List<byte[]> result = JsonSerializer.Deserialize<List<byte[]>>(message.ToString()) ?? new List<byte[]>();
                printer.PrintQueue(result);
                _logger.LogInformation("Received message {Channel} {Message}", channel, message);
            });
        }
    }
}