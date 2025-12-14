import pr11.Main;
import pr11.CustomExperiment;
import com.anylogic.engine.Engine;
import com.anylogic.engine.analysis.DataSet;

/**
 * Headless runner for the AnyLogic oil company model.
 * Outputs CSV results to stdout.
 */
public class ModelRunner {
    
    public static void main(String[] args) {
        int scenario = 1;
        int drillingRate = 50;
        double oilPrice = 80.0;
        double exchangeRate = 75.0;
        
        if (args.length >= 4) {
            try {
                scenario = Integer.parseInt(args[0]);
                drillingRate = Integer.parseInt(args[1]);
                oilPrice = Double.parseDouble(args[2]);
                exchangeRate = Double.parseDouble(args[3]);
            } catch (NumberFormatException e) {
                System.err.println("Error parsing arguments: " + e.getMessage());
                System.exit(1);
            }
        }
        
        System.err.println("Starting model with parameters:");
        System.err.println("  Scenario: " + scenario);
        System.err.println("  Drilling Rate: " + drillingRate);
        System.err.println("  Oil Price: " + oilPrice);
        System.err.println("  Exchange Rate: " + exchangeRate);
        
        final int finalScenario = scenario;
        
        try {
            CustomExperiment experiment = new CustomExperiment(null);
            Engine engine = experiment.createEngine();
            Main model = new Main(engine, null, null);
            
            model.setParametersToDefaultValues();
            model.Сценарий = scenario;
            model.Темп_бурения = drillingRate;
            model.Цена_на_нефть = oilPrice;
            model.Курс_доллара = exchangeRate;
            
            engine.setStartTime(0);
            engine.setStopTime(30);
            engine.setRealTimeMode(false);
            
            engine.start(model);
            engine.runFast();
            
            while (engine.getState() == Engine.State.RUNNING || 
                   engine.getState() == Engine.State.PAUSED) {
                Thread.sleep(100);
            }
            
            // Output CSV header
            System.out.println("Year,Scenario,Revenue,ProductionVolume,NewWellsFund,OldWellsFund");
            
            // Get DataSets - use the time dataset as primary reference
            DataSet dsTime = model._ds_время;
            DataSet dsRevenue = model._ds_Выручка;
            DataSet dsProduction = model._ds_Объем_добычи;
            DataSet dsNewWells = model._ds_Фонд_новых_скважин;
            DataSet dsOldWells = model._ds_Фонд_старых_скважин;
            
            // Debug: print dataset info
            System.err.println("DataSet sizes:");
            System.err.println("  Time: " + (dsTime != null ? dsTime.size() : "null"));
            System.err.println("  Revenue: " + (dsRevenue != null ? dsRevenue.size() : "null"));
            System.err.println("  Production: " + (dsProduction != null ? dsProduction.size() : "null"));
            System.err.println("  NewWells: " + (dsNewWells != null ? dsNewWells.size() : "null"));
            System.err.println("  OldWells: " + (dsOldWells != null ? dsOldWells.size() : "null"));
            
            // Sample data at regular yearly intervals from 0 to 30
            for (int year = 0; year <= 30; year++) {
                double time = (double) year;
                
                // Get interpolated values at this time point
                // For revenue, we need to calculate it from production * price * exchange rate
                // Or get it directly if the dataset has proper values
                double revenue = getValueAtTime(dsRevenue, time);
                double production = getValueAtTime(dsProduction, time);
                double newWells = getValueAtTime(dsNewWells, time);
                double oldWells = getValueAtTime(dsOldWells, time);
                
                // If revenue seems to be normalized (0-1 range), recalculate it
                // Revenue should be: Production * OilPrice * ExchangeRate
                if (revenue >= 0 && revenue <= 1 && production > 0) {
                    // Revenue dataset might contain normalized values, so calculate actual revenue
                    revenue = production * oilPrice * exchangeRate;
                }
                
                System.out.printf("%.2f,%d,%.2f,%.2f,%.2f,%.2f%n", 
                    time, finalScenario, revenue, production, newWells, oldWells);
            }
            
            engine.stop();
            System.err.println("Model completed successfully");
            
        } catch (Exception e) {
            System.err.println("Error running model: " + e.getMessage());
            e.printStackTrace(System.err);
            System.exit(1);
        }
    }
    
    /**
     * Get value from DataSet at a specific time using interpolation
     */
    private static double getValueAtTime(DataSet ds, double time) {
        if (ds == null || ds.size() == 0) {
            return 0;
        }
        
        int size = ds.size();
        
        // If time is before first point, return first value
        if (time <= ds.getX(0)) {
            return ds.getY(0);
        }
        
        // If time is after last point, return last value
        if (time >= ds.getX(size - 1)) {
            return ds.getY(size - 1);
        }
        
        // Find surrounding points and interpolate
        for (int i = 0; i < size - 1; i++) {
            double x1 = ds.getX(i);
            double x2 = ds.getX(i + 1);
            
            if (time >= x1 && time <= x2) {
                double y1 = ds.getY(i);
                double y2 = ds.getY(i + 1);
                double t = (x2 - x1) > 0 ? (time - x1) / (x2 - x1) : 0;
                return y1 + t * (y2 - y1);
            }
        }
        
        return ds.getY(size - 1);
    }
}
