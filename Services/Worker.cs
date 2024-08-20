using System.Text.Json;
using ESCPOS_NET.Emitters;
using StackExchange.Redis;

namespace PrintService.Services;

public class Worker : BackgroundService
{
    private readonly ILogger<Worker> _logger;
    private readonly IConfiguration _config;
    public Worker(ILogger<Worker> logger, IConfiguration config)
    {
        _logger = logger;
        _config = config;
        _connection = ConnectionMultiplexer.Connect(_config.GetConnectionString("Redis") ?? "localhost:6379");
    }

    private readonly ConnectionMultiplexer _connection;
    private static readonly RedisChannel channel = new RedisChannel("test-channel", RedisChannel.PatternMode.Auto);

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var subscriber = _connection.GetSubscriber();

        while (!stoppingToken.IsCancellationRequested)
        {
            _logger.LogInformation("Worker running at: {time}", DateTimeOffset.Now);
            List<byte[]> printInstructions = new List<byte[]>();
            EPSON e = new EPSON();
            printInstructions.Add(e.Print($"Worker running at: {DateTimeOffset.Now}"));
            string printInstructionsSerialized = JsonSerializer.Serialize(printInstructions);
            await subscriber.PublishAsync(channel, printInstructionsSerialized);
            await Task.Delay(5000, stoppingToken);
        }
    }
}