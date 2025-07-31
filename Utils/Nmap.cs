using System.Text;
using CliWrap;
using CliWrap.Buffered;
using PrintService.Models;

namespace PrintService.Utils;

public static class NMap
{
    public static async Task Scan(string channel, PrintMessage printMessage, ILogger logger, RedisClient redisClient, CancellationToken ct)
    {
        try
        {
            logger.LogInformation($"Starting nmap scan for {channel}");
            await redisClient.SetValueAsync($"status-{channel}", $"NMap Scan start Time: {DateTime.Now}");

            var ip = printMessage.IPAddress;
            var ipPattern = System.Text.RegularExpressions.Regex.Replace(ip, @"\d+$", "*");

            StringBuilder sb = new();
            IEnumerable<string> args = ["--open", $"-p{printMessage.Port}", ipPattern];
            string argsString = string.Join(" ", args);

            var result = await Cli.Wrap("nmap")
                .WithArguments(args)
                .ExecuteBufferedAsync(ct);

            sb.AppendLine($"nmap {argsString}");
            sb.AppendLine();
            sb.AppendLine($"Start Time: {result.StartTime}");
            sb.AppendLine($"Run Time: {result.RunTime}");
            sb.AppendLine($"Exit Code: {result.ExitCode}");
            sb.AppendLine();
            sb.AppendLine(result.StandardOutput);
            sb.AppendLine(result.StandardError);

            await redisClient.SetValueAsync($"result-{channel}", sb.ToString());
            logger.LogInformation($"Finished nmap scan for {channel}");
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Error Scanning");
        }
        finally
        {
            await redisClient.DeleteAsync($"status-{channel}");
        }
    }
}