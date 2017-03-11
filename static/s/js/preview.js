(function(app) {

  // always set Y axis to start at zero
  var options = {
    scales: {
      yAxes: [{
        display: true,
        ticks: {
          beginAtZero: true, // minimum value will be 0.
        }
      }]
    }
  }

  // do the hourly chart
  var hourChart = new Chart("chart-hour", {
    type    : 'bar',
    data    : app.hourData,
    options : options,
  })

  // do the DotW chart
  var dotwChart = new Chart("chart-dotw", {
    type    : 'bar',
    data    : app.dotwData,
    options : options,
  })

}(__POW__))
