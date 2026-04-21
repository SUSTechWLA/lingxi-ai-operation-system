package com.lingxi.ai.worker.tool.builtin;

import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolContext;
import com.lingxi.ai.worker.tool.ToolResult;
import com.lingxi.ai.worker.tool.ToolType;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.HashMap;
import java.util.Map;
import java.util.Random;

/**
 * 天气查询工具
 * 示例工具类，模拟查询指定城市的天气信息
 */
@Slf4j
@Component
public class WeatherTool implements Tool {

    private static final Map<String, String> WEATHER_DATA = new HashMap<>();
    private static final Random RANDOM = new Random();

    static {
        WEATHER_DATA.put("北京", "晴，温度 18-25℃，东南风 3级");
        WEATHER_DATA.put("上海", "多云，温度 20-28℃，东北风 2级");
        WEATHER_DATA.put("广州", "小雨，温度 25-32℃，西南风 2级");
        WEATHER_DATA.put("深圳", "多云转晴，温度 24-30℃，东风 3级");
        WEATHER_DATA.put("杭州", "晴，温度 17-24℃，北风 2级");
        WEATHER_DATA.put("成都", "阴，温度 16-22℃，南风 1级");
        WEATHER_DATA.put("武汉", "多云，温度 19-26℃，北风 2级");
        WEATHER_DATA.put("南京", "晴，温度 18-25℃，东风 2级");
    }

    @Override
    public String getName() {
        return "weather";
    }

    @Override
    public String getDescription() {
        return "查询指定城市的天气信息，支持北京、上海、广州、深圳、杭州、成都、武汉、南京等城市";
    }

    @Override
    public ToolType getType() {
        return ToolType.CUSTOM;
    }

    @Override
    public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
        Instant startTime = Instant.now();

        try {
            String city = (String) parameters.get("city");
            if (city == null || city.trim().isEmpty()) {
                return ToolResult.failure("城市名称不能为空", startTime, Instant.now());
            }

            log.info("查询天气: taskId={}, city={}", context.getTaskId(), city);

            // 模拟网络延迟
            Thread.sleep(100 + RANDOM.nextInt(200));

            String weather = WEATHER_DATA.get(city);
            if (weather == null) {
                // 未知城市返回默认天气
                weather = generateRandomWeather(city);
            }

            Map<String, Object> result = new HashMap<>();
            result.put("city", city);
            result.put("weather", weather);
            result.put("queryTime", Instant.now().toString());
            result.put("temperature", extractTemperature(weather));
            result.put("humidity", 50 + RANDOM.nextInt(40) + "%");

            log.info("天气查询成功: city={}, weather={}", city, weather);

            return ToolResult.success(result, startTime, Instant.now());

        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            log.error("天气查询被中断", e);
            return ToolResult.failure("查询被中断: " + e.getMessage(), startTime, Instant.now());
        } catch (Exception e) {
            log.error("天气查询失败", e);
            return ToolResult.failure("查询失败: " + e.getMessage(), startTime, Instant.now());
        }
    }

    @Override
    public boolean validateParameters(Map<String, Object> parameters) {
        return parameters != null && parameters.containsKey("city") && parameters.get("city") != null;
    }

    private String generateRandomWeather(String city) {
        String[] weathers = {"晴", "多云", "阴", "小雨", "阵雨", "雷阵雨"};
        int tempLow = 10 + RANDOM.nextInt(15);
        int tempHigh = tempLow + 5 + RANDOM.nextInt(10);
        String[] directions = {"东", "南", "西", "北", "东北", "东南", "西北", "西南"};
        String direction = directions[RANDOM.nextInt(directions.length)];
        int windLevel = 1 + RANDOM.nextInt(4);

        return String.format("%s，温度 %d-%d℃，%s风 %d级",
                weathers[RANDOM.nextInt(weathers.length)],
                tempLow, tempHigh, direction, windLevel);
    }

    private String extractTemperature(String weather) {
        if (weather == null) return "N/A";
        int start = weather.indexOf("温度");
        if (start == -1) return "N/A";
        int end = weather.indexOf("℃", start);
        if (end == -1) return "N/A";
        return weather.substring(start + 3, end + 1).trim();
    }
}
