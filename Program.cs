using PrintService.Extensions;
using PrintService.Services;

var builder = WebApplication.CreateBuilder(args);
builder.Host.AddLoggingConfiguration(builder.Configuration);
builder.Services.ConfigureConfig(builder.Configuration);

builder.Services.AddStackExchangeRedisCache(o =>
{
    o.Configuration = builder.Configuration.GetConnectionString("Redis");
});

builder.Services.AddHostedService<Worker>();
builder.Services.AddHostedService<Subscriber>();
builder.Services.AddSingleton<Printer>();
builder.Services.ConfigureSubscriber(builder.Configuration);

var app = builder.Build();

app.Run();
