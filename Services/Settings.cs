using Microsoft.Extensions.Options;
using PrintService.Config;

namespace PrintService.Services;

public class Settings
{
    private readonly MainConfig _config;
    private PrintersConfig? _printersConfig;
    public Settings(IOptions<MainConfig> config)
    {
        _config = config.Value;
    }

    public PrintersConfig? GetPrintersConfig()
    {
        if (_printersConfig != null)
        {
            return _printersConfig;
        }

        if (File.Exists(_config.Path))
        {
            string fileText = File.ReadAllText(_config.Path);
            PrintersConfig result = Utils.Yaml.GetDeserializer().Deserialize<PrintersConfig>(fileText);
            _printersConfig = result;
            var what = Utils.Yaml.GetSerializer().Serialize(result); // test result
            return result;
        }
        return null;
    }
}