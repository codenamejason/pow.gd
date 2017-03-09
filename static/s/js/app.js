var clipboard = new Clipboard('.btn-copy')
  .on('success', function(data) {
    console.log('Copied ' + data.text)
  })
  .on('error', function(err) {
    console.log('Err:', err)
  })
