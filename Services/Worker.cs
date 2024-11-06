using System.Text.Json;
using ESCPOS_NET.Emitters;
using PrintService.Models;
using PrintService.Utils;
using StackExchange.Redis;

namespace PrintService.Services;

public class Worker : BackgroundService
{
    private readonly ILogger<Worker> _logger;
    private readonly RedisClient _redisClient;
    public Worker(ILogger<Worker> logger, RedisClient redisClient)
    {
        _logger = logger;
        _redisClient = redisClient;
    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var subscriber = await _redisClient.GetSubscriber();
        RedisChannel channel = new RedisChannel($"print:2:1", RedisChannel.PatternMode.Auto);

        while (!stoppingToken.IsCancellationRequested)
        {
            _logger.LogInformation("Worker running at: {time}", DateTimeOffset.Now);

            List<byte[]> printInstructions = new List<byte[]>();
            EPSON e = new EPSON();
            printInstructions.Add(e.Print($"Worker running at: {DateTimeOffset.Now}"));

            PrintMessage printMessage = new()
            {
                IPAddress = "244.178.44.111",
                Port = 9100,
                PrintInstructions = printInstructions
            };
            string printInstructionsSerialized = JsonSerializer.Serialize(printMessage);

            await subscriber.PublishAsync(channel, printInstructionsSerialized);
            await Task.Delay(15000, stoppingToken);
        }
    }
}