using System.Text.Json;
using Microsoft.Extensions.Options;
using PrintService.Models;
using PrintService.Utils;
using StackExchange.Redis;

namespace PrintService.Services;

public class Subscriber : BackgroundService
{
    private readonly ILogger<Worker> _logger;
    private readonly RedisClient _redisClient;
    private readonly Config _config;

    public Subscriber(ILogger<Worker> logger, RedisClient redisClient, IOptions<Config> config)
    {
        _logger = logger;
        _redisClient = redisClient;
        _config = config.Value;
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        try
        {
            foreach (var outletId in _config.OutletIds)
            {
                _logger.LogInformation("Printer Subscriber started and listening for {channel}", $"print:{outletId}:{_config.DeviceId}");
                RedisChannel channel = new RedisChannel($"print:{outletId}:{_config.DeviceId}", RedisChannel.PatternMode.Auto);
                var subscriber = await _redisClient.GetSubscriber();
                await subscriber.SubscribeAsync(channel, async (channel, message) =>
                {
                    if (message.IsNullOrEmpty)
                    {
                        return;
                    }
                    _logger.LogInformation("Received message {Channel} {Message}", channel, message);
                    PrintMessage? printMessage = JsonSerializer.Deserialize<PrintMessage>(message.ToString());
                    if (printMessage == null)
                    {
                        _logger.LogInformation("Received empty message: {Channel} {Message}", channel, message);
                        return;
                    }
                    await Printer.Print(printMessage, _logger);
                });
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Error in Subscriber");
        }
    }

}