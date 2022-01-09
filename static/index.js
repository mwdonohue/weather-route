let markers = [];

function initMap() {
  const directionsRenderer = new google.maps.DirectionsRenderer();
  const map = new google.maps.Map(document.getElementById("map"), {
    zoom: 7,
    center: {
      lat: 35.77,
      lng: -78.6382
    },
  });

  directionsRenderer.setMap(map);

  const sourceInput = document.getElementById("source-input")
  const destInput = document.getElementById("destination-input")

  $('#source-input-container #source-input').typeahead({
    autoselect: true,
  }, {
    source: function (query, syncResults, asyncResults) {
      $.ajax({
        type: "POST",
        url: "/autoCompleteSuggestions",
        data: JSON.stringify({ placeToAutoComplete: query }),
        dataType: "json",
        contentType: "application/json"
      }).done((data, textStatus, request) => {
        asyncResults(data)
      }).fail((request, textStatus, errorThrown) => {
        console.log(errorThrown)
      })
    }
  })

  $('#destination-input-container #destination-input').typeahead({
    autoselect: true,
  }, {
    source: function (query, syncResults, asyncResults) {
      $.ajax({
        type: "POST",
        url: "/autoCompleteSuggestions",
        data: JSON.stringify({ placeToAutoComplete: query }),
        dataType: "json",
        contentType: "application/json"
      }).done((data, textStatus, request) => {
        asyncResults(data)
      }).fail((request, textStatus, errorThrown) => {
        console.log(errorThrown)
      })
    }
  })

  document.getElementById("get-route-button").addEventListener("click", () => {
    if (sourceInput.value.length != 0 && destInput.value.length != 0) {
      for (let i = 0; i < markers.length; i++) {
        markers[i].setMap(null)
      }
      markers = []
      calculateAndDisplayRoute(map, directionsRenderer, sourceInput.value, destInput.value);
    }
  })
}

function calculateAndDisplayRoute(map, directionsRenderer, origin, dest) {
  postData('/directions', {
    origin: origin,
    destination: dest
  }).then(resp => {
    return resp.json()
  }).then(data => {
    newData = addGoogleServiceSDKFields(data)
    newData['request'] = {
      travelMode: 'DRIVING'
    }
    directionsRenderer.setDirections(
      newData
    )

    postData('/weather', data.routes).then(
      resp => resp.json()
    ).then(weatherpoints => {
      for (let i = 0; i < weatherpoints.length; i++) {
        let weatherpoint = weatherpoints[i]
        const marker = new google.maps.Marker({
          position: {
            lat: weatherpoint.coordinate.lat,
            lng: weatherpoint.coordinate.lng
          },
          map,
          icon: "https://openweathermap.org/img/wn/" + weatherpoint.weatherData.weatherIcon + ".png"
        })
        const window = new google.maps.InfoWindow({
          content: "Temperature: " + weatherpoint.weatherData.temperature.toString() + "<br>" +
            "Chance of Precip: " + (100 * weatherpoint.weatherData.precipChance).toString() + "%" + "<br>" +
            "At: " + (() => {
              pointTime = new Date(weatherpoint.time)
              ampm = "AM"
              totalHours = pointTime.getHours()

              if (totalHours > 12) {
                totalHours -= 12
                ampm = "PM"
              } else if (totalHours == 12) {
                ampm = "PM"
              }
              return totalHours.toString() + " " + ampm
            })()
        })
        marker.addListener("click", () => {
          window.open({
            anchor: marker,
            map,
            shouldFocus: false,
          })
        })
        markers.push(marker)
      }
    })
  })
}
async function postData(url = '', data = {}) {
  const response = fetch(url, {
    method: 'POST',
    mode: 'cors',
    cache: 'no-cache',
    headers: {
      'Content-Type': 'application/json'
    },
    redirect: 'follow',
    referrerPolicy: 'no-referrer',
    body: JSON.stringify(data)
  });
  return response;
}

function removeAllChildNodes(parent) {
  while (parent.firstChild) {
    parent.removeChild(parent.firstChild);
  }
}

addGoogleServiceSDKFields = (serverResponse) => {
  serverResponse.routes = serverResponse.routes.map((response) => {
    const bounds = new google.maps.LatLngBounds(
      response.bounds.southwest,
      response.bounds.northeast,
    );
    response.bounds = bounds;
    response.overview_path =
      google.maps.geometry.encoding.decodePath(response.overview_polyline.points);

    response.legs = response.legs.map((leg) => {
      leg.start_location =
        new this.google.maps.LatLng(leg.start_location.lat, leg.start_location.lng);
      leg.end_location =
        new this.google.maps.LatLng(leg.end_location.lat, leg.end_location.lng);
      leg.steps = leg.steps.map((step) => {
        step.path = google.maps.geometry.encoding.decodePath(step.polyline.points);
        step.start_location = new google.maps.LatLng(
          step.start_location.lat,
          step.start_location.lng,
        );
        step.end_location = new google.maps.LatLng(
          step.end_location.lat,
          step.end_location.lng,
        );
        return step;
      });
      return leg;
    });

    return response;
  });

  return serverResponse;
}