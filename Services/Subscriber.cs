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

    private int _failureCount = 0;

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var subscriber = await _redisClient.GetSubscriber();
        await SubscribePrinters(subscriber, stoppingToken);

        // Check if connection is still active
        while (!stoppingToken.IsCancellationRequested)
        {
            await Task.Delay(10000, stoppingToken);
            if (subscriber.IsConnected() == false)
            {
                _failureCount++;
            }
            else
            {
                _failureCount = 0;
            }

            if (_failureCount > 5)
            {
                _logger.LogInformation("Concurrent failure count reached. Restarting subscriber");
                _failureCount = 0;
                await subscriber.UnsubscribeAllAsync();
                await SubscribePrinters(subscriber, stoppingToken);
            }
        }
    }

    private async Task SubscribePrinters(ISubscriber subscriber, CancellationToken stoppingToken)
    {
        bool isError = false;
        foreach (var outletId in _config.OutletIds.Split(","))
        {
            _logger.LogInformation(
                "Printer Subscriber started and listening for {channel}",
                $"print:{outletId}:{_config.DeviceId}"
            );
            RedisChannel channel = new RedisChannel(
                $"print:{outletId}:{_config.DeviceId}",
                RedisChannel.PatternMode.Auto
            );
            try
            {
                await subscriber.SubscribeAsync(
                    channel,
                    async (channel, message) =>
                    {
                        if (message.IsNullOrEmpty)
                        {
                            return;
                        }
                        _logger.LogInformation(
                            "Received message {Channel} {Message}",
                            channel,
                            message
                        );
                        try
                        {
                            PrintMessage? printMessage = JsonSerializer.Deserialize<PrintMessage>(
                                message.ToString()
                            );
                            if (printMessage == null)
                            {
                                _logger.LogInformation(
                                    "Received empty message: {Channel} {Message}",
                                    channel,
                                    message
                                );
                                return;
                            }
                            if (printMessage.Action == "nmap")
                            {
                                await NMap.Scan(channel.ToString(), printMessage, _logger, _redisClient, stoppingToken);
                            }
                            else
                            {
                                await Printer.Print(printMessage, _logger);
                            }
                        }
                        catch (Exception ex)
                        {
                            _logger.LogError(ex, "Error with print instructions");
                        }
                    }
                );
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Error in SubscribePrinters");
                isError = true;
            }
        }
        if (isError)
        {
            _logger.LogInformation("Failed to subscribe to all printers");
        }
    }
}
