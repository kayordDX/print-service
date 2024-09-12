using Microsoft.Extensions.Options;
using PrintService.Config;

namespace PrintService.Services;

public class Settings
{
    private readonly MainConfig _config;
    private PrintersConfig _printersConfig = new PrintersConfig();
    public Settings(IOptions<MainConfig> config)
    {
        _config = config.Value;
    }

    public PrintersConfig GetPrintersConfig()
    {
        if (File.Exists(_config.Path))
        {
            string fileText = File.ReadAllText(_config.Path);
            PrintersConfig result = Utils.Yaml.GetDeserializer().Deserialize<PrintersConfig>(fileText);
            _printersConfig = result;
            return result;
        }
        return _printersConfig;
    }
}