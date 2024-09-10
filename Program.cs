using PrintService.Extensions;
using PrintService.Services;

var builder = WebApplication.CreateBuilder(args);
builder.Host.AddLoggingConfiguration(builder.Configuration);
builder.Services.ConfigureConfig(builder.Configuration);
builder.Services.ConfigureRedis(builder.Configuration);
builder.Services.AddSingleton<Settings>();
builder.Services.AddSingleton<Printers>();
builder.Services.AddHostedService<PrinterCheck>();
builder.Services.AddHostedService<PrinterBackground>();
builder.Services.AddHostedService<Subscriber>();
builder.Services.ConfigureSubscriber(builder.Configuration);

var app = builder.Build();

app.Run();
