using PrintService.Extensions;
using PrintService.Services;

var builder = WebApplication.CreateBuilder(args);
builder.Host.AddLoggingConfiguration(builder.Configuration);
builder.Services.ConfigureConfig(builder.Configuration);
builder.Services.ConfigureRedis(builder.Configuration);
builder.Services.AddHostedService<Subscriber>();
builder.Services.AddHostedService<Worker>();
builder.Services.ConfigureSubscriber(builder.Configuration);

var app = builder.Build();

app.Run();
