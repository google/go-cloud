# Wire Tutorial

Let's learn to use Wire by example. The [Wire README][readme] provides thorough
documentation of the tool's usage. For readers eager to jump right in, the
[guestbook sample][guestbook] uses Wire to initialize its components. Here we
are going to build a small greeter CLI to understand how to use Wire. The
finished product may be found in the same directory as this README.

As a first pass, let's say we will create three types: 1) an event, 2) a
greeter to greet guests at the event, and 3) a message that the greeter will
say to each guest. In this design, we have three `struct` types:

``` go
type Message string

type Greeter struct {
    // ... TBD
}

type Event struct {
    // ... TBD
}
```

The `Message` type just wraps a string. Our `Greeter` will need reference to
it. So let's create an initializer for our `Greeter`.

``` go
func NewGreeter(m Message) Greeter {
    return Greeter{Message: m}
}
```

In the intializer we assign a `Message` field to `Greeter`. Now, we can use the
`Message` when we create a `Greet` method on `Greeter`:

``` go
func (g Greeter) Greet() Message {
    return g.Message
}
```

Next, we need our `Event` to have a `Greeter`, so we will create an initializer
for it as well.

``` go
func NewEvent(g Greeter) Event {
    return Event{Greeter: g}
}
```

Then we add a method to start the `Event`:

``` go
func (e Event) Start() {
    msg := e.Greeter.Greet()
    fmt.Println(msg)
}
```

The `Start` method holds the core of our small application: it tells the
greeter to issue a greeting and then prints that message to the screen.

Now that we have all the components of our application ready, let's wire them
together in a `main` function:

``` go
func main() {
    message := NewMessage()
    greeter := NewGreeter(message)
    event := NewEvent(greeter)

    event.Start()
}
```

First we create a message, then we create a greeter with that message, and
finally we create an event with that greeter. With all the initialization done,
we're ready to start our event.

In the design here, we are using dependency injection. We pass in whatever each
component needs. This style of design lends itself to writing easily tested
code and makes it easy to swap out one dependency with another. The idea of
dependency injection isn't much more complicated than this.

One downside to dependency injection is the need for so many initialization
steps. Let's see how we can use Wire to make the process of initailizing our
components smoother.

Let's start by changing our `main` function to look like this:

``` go
func main() {
    e := InitializeEvent()

    e.Start()
}
```

Next, in a separate file called `wire.go` we'll define `InitializeEvent` and
this is where things get interesting:

``` go
// wire.go

func InitializeEvent() Event {
    wire.Build(NewEvent, NewGreeter, NewMessage)
    return Event{}
}
```

Rather than go through the trouble of initializing each component in turn and
passing it into the next one, we instead have a single call to `wire.Build`
passing in the initializers we want to use. In Wire, initializers are known as
"providers," functions which provide a particular type. We add a zero value for
`Event` as a return value to satisfy the compiler. This won't be included in
our final binary so we add a build constraint to the top of the file:

``` go
//+build wireinject

```

Note, a [build constraint][constraint] requires a blank, trailing line.

In Wire parlance, `InitializeEvent` is an injector. Now that we have our
injector complete, we are ready to use the `wire` command line tool.

Install the tool with:

``` shell
go get github.com/google/go-cloud/wire/cmd/wire
```

Then in the same directory with the above code, simply run `wire`. Wire will
find the `InitializeEvent` injector and generate a function whose body is
filled out with all the necessary initialization steps. The result will be
written to a file named `wire_gen.go`.

Let's take a look at what Wire did for us:

``` go
// wire_gen.go

func InitializeEvent() Event {
    message := NewMessage()
    greeter := NewGreeter(message)
    event := NewEvent(greeter)
    return event
}
```

It looks just like what we wrote above! Now this is a simple example with just
three components, so writing the initializer by hand isn't too painful. Imagine
how useful Wire is for components that are much more complex.

To show a small part of how Wire handles more complex setups, let's refactor
our initializer for `Event` to return an error and see what happens.

``` go
func NewEvent(g Greeter) (Event, error) {
    if g.Deranged {
        return Event{}, errors.New("could not create event: event greeter is deranged")
    }
    return Event{Greeter: g}, nil
}
```

We'll say that sometimes a `Greeter` might be deranged and so we cannot create
an `Event`. The `NewGreeter` initializer now looks like this:

``` go
func NewGreeter(m Message) Greeter {
    var deranged bool
    if time.Now().Unix()%2 == 0 {
        deranged = true
    }
    return Greeter{Message: m, Deranged: deranged}
}
```

We have added a `Deranged` struct field to `Greeter` and if the invocation time
of the initializer is an even number of seconds since the epoch, we will create
a deranged greeter instead of a friendly one.

The `Greet` method then becomes:

``` go
func (g Greeter) Greet() Message {
    if g.Deranged {
        return Message("Go away!")
    }
    return g.Message
}
```

Now you see how a deranged `Greeter` is no good for an `Event`. So `NewEvent`
may fail. Our `main` must now take into account that `InitializeEvent` may in
fact fail:

``` go
func main() {
    e, err := InitializeEvent()
    if err != nil {
        fmt.Printf("failed to create event: %s\n", err)
        os.Exit(2)
    }
    e.Start()
}
```

We also need to update `InitializeEvent` to add an `error` type to the return value:

``` go
// wire.go

func InitializeEvent() (Event, error) {
    wire.Build(NewEvent, NewGreeter, NewMessage)
    return Event{}, nil
}
```

With the setup complete, we are ready to invoke the `wire` command again. After
running the command, our `wire_gen.go` file looks like this:

``` go
// wire_gen.go

func InitializeEvent() (Event, error) {
    message := NewMessage()
    greeter := NewGreeter(message)
    event, err := NewEvent(greeter)
    if err != nil {
        return Event{}, err
    }
    return event, nil
}
```

Wire has detected that the `NewEvent` provider may fail and has done the right
thing inside the generated code: it checks the error and returns early if one
is present.

As another improvement, let's look at how Wire generates code based on the
signature of the injector. Presently, we have hard-coded the message inside
`NewMessage`. In practice, it's much nicer to allow callers to change that
message however they see fit. So let's change `InitializeEvent` to look like this:

``` go
func InitializeEvent(phrase string) (Event, error) {
    wire.Build(NewEvent, NewGreeter, NewMessage)
    return Event{}, nil
}
```

Now `InitializeEvent` allows callers to pass in the `phrase` for a `Greeter` to
use.

After we run `wire` again, we will see that the tool has generated an
initializer which passes the `phrase` value as a `Message` into `Greeter`.
Neat!

Let's summarize what we have done here. First, we wrote a number of components
with corresponding initializers, or providers. Next, we created an injector
function, specifying which arguments it receives and which types it returns.
Then, we filled in the injector function with a call to `wire.Build` supplying
all necessary providers. Finally, we ran the `wire` command to generate code
that wires up all the different initializers. When we added an argument to the
injector and an error return value, running `wire` again made all the necessary
updates to our generate code.

The example here is small, but it demonstrates some of the power of Wire, and
how it takes much of the pain out of initializing code using dependency
injection. Furthermore, using Wire produced code that looks much like what we
would otherwise write. There are no bespoke types that commit a user to Wire.
Instead it's just generated code. We may do with it what we will.

In closing, it is worth mentioning that Wire supports a number of additional
features not discussed here. Providers may be grouped in [provider sets][sets].
There is support for [binding interfaces][interfaces], [binding
values][values], as well as support for [cleanup functions][cleanup]. See the
[Advanced Features][advanced] section for more.

[readme]: https://github.com/google/go-cloud/blob/master/wire/README.md
[guestbook]: https://github.com/google/go-cloud/tree/master/samples/guestbook
[constraint]: https://godoc.org/go/build#hdr-Build_Constraints
[sets]: https://github.com/google/go-cloud/tree/master/wire#defining-providers
[interfaces]: https://github.com/google/go-cloud/tree/master/wire#binding-interfaces
[values]: https://github.com/google/go-cloud/tree/master/wire#binding-values
[cleanup]: https://github.com/google/go-cloud/tree/master/wire#cleanup-functions
[advanced]: https://github.com/google/go-cloud/tree/master/wire#advanced-features
