using System.Text.Json;
using Microsoft.Extensions.Options;
using PrintService.Config;
using StackExchange.Redis;

namespace PrintService.Services;

public class Subscriber : BackgroundService
{
    private readonly ILogger<Worker> _logger;
    private readonly Printer _printer;
    private readonly RedisClient _redisClient;
    private readonly PrinterConfig _printerConfig;

    public Subscriber(ILogger<Worker> logger, RedisClient redisClient, Printer printer, IOptions<PrinterConfig> printerConfig)
    {
        _logger = logger;
        _redisClient = redisClient;
        _printer = printer;
        _printerConfig = printerConfig.Value;
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        RedisChannel channel = new RedisChannel($"print:{_printerConfig.OutletId}:{_printerConfig.PrinterId}", RedisChannel.PatternMode.Auto);
        var subscriber = await _redisClient.GetSubscriber();

        await subscriber.SubscribeAsync(channel, (channel, message) =>
        {
            if (message.IsNullOrEmpty)
            {
                return;
            }

            List<byte[]> result = JsonSerializer.Deserialize<List<byte[]>>(message.ToString()) ?? new List<byte[]>();
            _printer.PrintQueue(result);
            _logger.LogInformation("Received message {Channel} {Message}", channel, message);
        });
    }
}