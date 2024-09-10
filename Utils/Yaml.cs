using YamlDotNet.Serialization;
using YamlDotNet.Serialization.NamingConventions;

namespace PrintService.Utils;

public static class Yaml
{
    public static ISerializer GetSerializer()
    {
        return new SerializerBuilder()
            .WithNamingConvention(CamelCaseNamingConvention.Instance)
            .ConfigureDefaultValuesHandling(DefaultValuesHandling.OmitNull)
            .Build();
    }

    public static IDeserializer GetDeserializer()
    {
        return new DeserializerBuilder()
            .IgnoreUnmatchedProperties()
            .WithNamingConvention(CamelCaseNamingConvention.Instance).Build();
    }
}