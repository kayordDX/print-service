using System.Text.Json;
using ESCPOS_NET.Emitters;
using StackExchange.Redis;

namespace PrintService.Services;

public class Subscriber : BackgroundService
{
    private readonly ILogger<Worker> _logger;
    private readonly Printer _printer;
    private readonly IConfiguration _config;

    public Subscriber(ILogger<Worker> logger, IConfiguration config, Printer printer)
    {
        _logger = logger;
        _config = config;
        _connection = ConnectionMultiplexer.Connect(_config.GetConnectionString("Redis") ?? "localhost:6379");
        _printer = printer;
    }

    private readonly ConnectionMultiplexer _connection;
    private static readonly RedisChannel channel = new RedisChannel("test-channel", RedisChannel.PatternMode.Auto);

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var subscriber = _connection.GetSubscriber();

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